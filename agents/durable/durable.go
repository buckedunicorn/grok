// Package durable provides checkpoint persistence for agents.Runner.
//
// The package's central concept is Store, a persistent backend that
// satisfies agents.Checkpointer (for the Runner side) and adds Load,
// List, and Delete operations for management. Two implementations ship:
// MemoryStore for tests and single-process workloads, and FileStore for
// disk-backed persistence (atomic JSON writes via temp+rename).
//
// Typical usage:
//
//	store, _ := durable.NewFileStore("/var/lib/grok-runs")
//	runner := &agents.Runner{
//	 Client: client.Chat,
//	 Checkpointer: store,
//	}
//
//	// First attempt, runs and checkpoints after each tool turn.
//	res, err := runner.Run(ctx, agent, agents.RunOptions{Input: "..."})
//	if err == nil {
//	 return res.Output, nil
//	}
//
//	// On a later restart, resume from the last checkpoint:
//	res, err = durable.Resume(ctx, runner, store, runID, agent)
//
// The Runner's Checkpointer interface only requires SaveCheckpoint, so
// callers wanting only the save side can implement that one method without
// pulling in any of the agents/durable code.
package durable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/buckedunicorn/grok/agents"
)

// ErrNotFound is returned by Store.Load when the runID has no checkpoint.
var ErrNotFound = errors.New("durable: checkpoint not found")

// Store persists CheckpointState. Implementations satisfy
// agents.Checkpointer (SaveCheckpoint) and add Load / List / Delete for
// management. All methods must be safe for concurrent use across runIDs;
// callers must not run two Save operations on the same runID concurrently.
type Store interface {
	agents.Checkpointer
	Load(ctx context.Context, runID string) (*agents.CheckpointState, error)
	Delete(ctx context.Context, runID string) error
	List(ctx context.Context) ([]string, error)
}

// Resume loads the checkpoint for runID and continues the run. The agent
// must be the one that was active at checkpoint time, durable does not
// look it up for you (and can't, since tool Handlers aren't serializable).
//
// On success, the returned RunResult has the same RunID as the checkpoint
// and a Trajectory containing every turn from before and after the resume.
func Resume(ctx context.Context, runner *agents.Runner, store Store, runID string, agent *agents.Agent) (*agents.RunResult, error) {
	if store == nil {
		return nil, errors.New("durable: store is nil")
	}
	if runner == nil {
		return nil, errors.New("durable: runner is nil")
	}
	state, err := store.Load(ctx, runID)
	if err != nil {
		return nil, err
	}
	return runner.Run(ctx, agent, agents.RunOptions{Resume: state})
}

// --- MemoryStore -----------------------------------------------------------

// MemoryStore keeps checkpoints in process memory. Suitable for tests and
// single-process workloads where durability across crashes is not required.
type MemoryStore struct {
	mu     sync.RWMutex
	states map[string]agents.CheckpointState
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{states: map[string]agents.CheckpointState{}}
}

// SaveCheckpoint satisfies agents.Checkpointer.
func (m *MemoryStore) SaveCheckpoint(_ context.Context, state agents.CheckpointState) error {
	m.mu.Lock()
	m.states[state.RunID] = state
	m.mu.Unlock()
	return nil
}

// Load satisfies Store.
func (m *MemoryStore) Load(_ context.Context, runID string) (*agents.CheckpointState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.states[runID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, runID)
	}
	// Return a copy so the caller's mutations don't affect storage.
	cp := state
	return &cp, nil
}

// Delete satisfies Store.
func (m *MemoryStore) Delete(_ context.Context, runID string) error {
	m.mu.Lock()
	delete(m.states, runID)
	m.mu.Unlock()
	return nil
}

// List satisfies Store. Returned IDs are in lexicographic order.
func (m *MemoryStore) List(_ context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.states))
	for id := range m.states {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// --- FileStore -------------------------------------------------------------

// FileStore persists checkpoints as JSON files under Dir. One file per
// runID, named "<runID>.json". Writes are atomic via temp+rename so a
// crash mid-write cannot corrupt an existing checkpoint.
//
// The runID is used directly as part of the filename, so it must be safe
// for the host filesystem. Runner-generated run IDs are 16-char hex
// strings (see agents.newRunID), which is always safe; if you supply
// custom IDs, restrict them to [A-Za-z0-9_-] to avoid surprises.
//
// All disk operations go through *os.Root (Go 1.24+). The kernel
// enforces directory containment, so even if validateRunID ever
// regressed, the filesystem itself prevents writes outside Dir.
type FileStore struct {
	Dir  string
	root *os.Root
	mu   sync.Mutex // serializes writes per process; cross-process locks are out of scope
}

// NewFileStore creates a FileStore rooted at dir, creating the directory if
// it does not exist. Returns an error if dir is empty or refers to an
// existing non-directory.
func NewFileStore(dir string) (*FileStore, error) {
	if dir == "" {
		return nil, errors.New("durable: FileStore dir is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("durable: create dir: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("durable: stat dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("durable: %q is not a directory", dir)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("durable: open root: %w", err)
	}
	return &FileStore{Dir: dir, root: root}, nil
}

// Close releases the os.Root handle. Optional for test workloads;
// long-lived processes typically leak this on shutdown.
func (s *FileStore) Close() error {
	if s == nil || s.root == nil {
		return nil
	}
	return s.root.Close()
}

// SaveCheckpoint satisfies agents.Checkpointer. Atomic via temp+rename.
func (s *FileStore) SaveCheckpoint(_ context.Context, state agents.CheckpointState) error {
	if state.RunID == "" {
		return errors.New("durable: cannot save state with empty RunID")
	}
	if err := validateRunID(state.RunID); err != nil {
		return err
	}
	// Compact JSON to keep per-checkpoint encoding cost down
	//. Pretty-printed output is not required for the
	// SDK's own consumers; on-disk inspection still works via jq.
	body, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("durable: marshal: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	finalName := state.RunID + ".json"
	tmpName := "." + state.RunID + ".tmp"

	tmp, err := s.root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("durable: create temp: %w", err)
	}
	cleanup := func() {
		if tmpName != "" {
			_ = s.root.Remove(tmpName)
		}
	}
	defer cleanup()

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("durable: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("durable: fsync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("durable: close temp: %w", err)
	}
	// Use the rooted Rename so the directory containment claimed by
	// the os.Root migration is actually enforced. A swap of Dir for a
	// symlink elsewhere on disk between OpenRoot and Rename cannot
	// redirect this rename; the kernel resolves both paths against
	// the held-open root handle.
	if err := s.root.Rename(tmpName, finalName); err != nil {
		return fmt.Errorf("durable: rename: %w", err)
	}
	tmpName = "" // disarm cleanup
	return nil
}

// Load satisfies Store.
func (s *FileStore) Load(_ context.Context, runID string) (*agents.CheckpointState, error) {
	if err := validateRunID(runID); err != nil {
		return nil, err
	}
	f, err := s.root.Open(runID + ".json")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, runID)
	}
	if err != nil {
		return nil, fmt.Errorf("durable: open: %w", err)
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("durable: read: %w", err)
	}
	var state agents.CheckpointState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("durable: parse %q: %w", runID, err)
	}
	return &state, nil
}

// Delete satisfies Store.
func (s *FileStore) Delete(_ context.Context, runID string) error {
	if err := validateRunID(runID); err != nil {
		return err
	}
	err := s.root.Remove(runID + ".json")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("durable: remove: %w", err)
	}
	return nil
}

// List satisfies Store. Returned IDs are in lexicographic order. Hidden
// files (starting with '.', including in-progress temp files) are skipped.
func (s *FileStore) List(_ context.Context) ([]string, error) {
	dir, err := s.root.Open(".")
	if err != nil {
		return nil, fmt.Errorf("durable: open dir: %w", err)
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("durable: list dir: %w", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(out)
	return out, nil
}

// validateRunID rejects path-traversal attempts and any control characters.
// Runner-generated IDs are hex; users supplying custom IDs should stick to
// [A-Za-z0-9_-].
func validateRunID(runID string) error {
	if runID == "" {
		return errors.New("durable: runID is empty")
	}
	if strings.ContainsAny(runID, `/\`) || strings.Contains(runID, "..") {
		return fmt.Errorf("durable: invalid runID %q", runID)
	}
	for _, r := range runID {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("durable: invalid runID %q (control char)", runID)
		}
	}
	return nil
}
