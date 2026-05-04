package durable_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/agents/durable"
	"github.com/buckedunicorn/grok/chat"
)

func mkState(runID string) agents.CheckpointState {
	return agents.CheckpointState{
		RunID:     runID,
		NextTurn:  3,
		AgentName: "Helper",
		Messages: []chat.Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
		},
		Trajectory: agents.Trajectory{
			RunID:     runID,
			StartedAt: time.Unix(1700000000, 0).UTC(),
			Turns:     []agents.Turn{{Index: 0, AgentName: "Helper"}},
		},
		Usage:   chat.Usage{TotalTokens: 42},
		SavedAt: time.Unix(1700000050, 0).UTC(),
	}
}

// --- MemoryStore ---

func TestMemoryStore_saveLoadRoundTrip(t *testing.T) {
	s := durable.NewMemoryStore()
	state := mkState("run-a")

	if err := s.SaveCheckpoint(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(context.Background(), "run-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != state.RunID || got.NextTurn != state.NextTurn || got.AgentName != state.AgentName {
		t.Errorf("mismatched state: %+v", got)
	}
	if got.Usage.TotalTokens != 42 {
		t.Errorf("usage lost: %+v", got.Usage)
	}
}

func TestMemoryStore_loadMissing(t *testing.T) {
	s := durable.NewMemoryStore()
	_, err := s.Load(context.Background(), "ghost")
	if !errors.Is(err, durable.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_listAndDelete(t *testing.T) {
	s := durable.NewMemoryStore()
	for _, id := range []string{"c", "a", "b"} {
		_ = s.SaveCheckpoint(context.Background(), mkState(id))
	}
	got, _ := s.List(context.Background())
	want := []string{"a", "b", "c"}
	if !slices.Equal(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
	_ = s.Delete(context.Background(), "b")
	got, _ = s.List(context.Background())
	if !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("after delete: %v", got)
	}
}

func TestMemoryStore_loadReturnsCopy(t *testing.T) {
	// Mutating a Loaded state must not affect storage.
	s := durable.NewMemoryStore()
	_ = s.SaveCheckpoint(context.Background(), mkState("x"))
	got, _ := s.Load(context.Background(), "x")
	got.AgentName = "MUTATED"
	got2, _ := s.Load(context.Background(), "x")
	if got2.AgentName == "MUTATED" {
		t.Error("Load returned mutable reference; mutations leaked into storage")
	}
}

func TestMemoryStore_concurrentSafeAcrossRunIDs(t *testing.T) {
	s := durable.NewMemoryStore()
	var wg sync.WaitGroup
	const n = 50
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a' + i%26))
			_ = s.SaveCheckpoint(context.Background(), mkState(id+"-x"))
			_, _ = s.Load(context.Background(), id+"-x")
			_, _ = s.List(context.Background())
		}(i)
	}
	wg.Wait()
}

// --- FileStore ---

func TestFileStore_saveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := durable.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := mkState("run-1")
	if err := s.SaveCheckpoint(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	// Verify on disk.
	body, err := os.ReadFile(filepath.Join(dir, "run-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip agents.CheckpointState
	if err := json.Unmarshal(body, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if roundtrip.RunID != "run-1" || roundtrip.AgentName != "Helper" {
		t.Errorf("on-disk JSON wrong: %+v", roundtrip)
	}

	got, err := s.Load(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != "run-1" || got.NextTurn != 3 {
		t.Errorf("Load = %+v", got)
	}
}

func TestFileStore_loadMissing(t *testing.T) {
	s, _ := durable.NewFileStore(t.TempDir())
	_, err := s.Load(context.Background(), "ghost")
	if !errors.Is(err, durable.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestFileStore_listSkipsTempAndNonJSON(t *testing.T) {
	dir := t.TempDir()
	s, _ := durable.NewFileStore(dir)
	_ = s.SaveCheckpoint(context.Background(), mkState("real"))

	// Drop a temp-style file and a non-JSON file in the dir.
	_ = os.WriteFile(filepath.Join(dir, ".real.123.tmp"), []byte("partial"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "README.txt"), []byte("notes"), 0o644)

	got, _ := s.List(context.Background())
	if !slices.Equal(got, []string{"real"}) {
		t.Errorf("List = %v, want [real]", got)
	}
}

func TestFileStore_delete(t *testing.T) {
	s, _ := durable.NewFileStore(t.TempDir())
	_ = s.SaveCheckpoint(context.Background(), mkState("gone"))
	if err := s.Delete(context.Background(), "gone"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Load(context.Background(), "gone")
	if !errors.Is(err, durable.ErrNotFound) {
		t.Errorf("after delete, Load err = %v, want ErrNotFound", err)
	}
	// Deleting a missing checkpoint must NOT error.
	if err := s.Delete(context.Background(), "ghost"); err != nil {
		t.Errorf("Delete on missing: %v", err)
	}
}

func TestFileStore_atomicOverwrite(t *testing.T) {
	// Save twice in succession, the second save must replace the first
	// without leaving a partially-written file. Sanity check: List
	// returns exactly one entry, and Load returns the second state.
	dir := t.TempDir()
	s, _ := durable.NewFileStore(dir)

	first := mkState("run-1")
	first.AgentName = "First"
	second := mkState("run-1")
	second.AgentName = "Second"
	second.NextTurn = 7

	if err := s.SaveCheckpoint(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCheckpoint(context.Background(), second); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Load(context.Background(), "run-1")
	if got.AgentName != "Second" || got.NextTurn != 7 {
		t.Errorf("after overwrite, state = %+v", got)
	}

	// Make sure no temp files leaked.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected 1 file after overwrite, got %v", names)
	}
}

func TestFileStore_rejectsTraversalRunID(t *testing.T) {
	s, _ := durable.NewFileStore(t.TempDir())
	bad := []string{"..", "../etc/passwd", "a/b", `c\d`, "with\x00null"}
	for _, id := range bad {
		state := mkState(id)
		err := s.SaveCheckpoint(context.Background(), state)
		if err == nil {
			t.Errorf("expected validation error for %q", id)
		}
	}
}

func TestFileStore_emptyDirError(t *testing.T) {
	if _, err := durable.NewFileStore(""); err == nil {
		t.Error("expected error for empty dir")
	}
}

func TestFileStore_dirPointsAtFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "iam-a-file")
	_ = os.WriteFile(tmp, []byte("x"), 0o644)
	if _, err := durable.NewFileStore(tmp); err == nil {
		t.Error("expected error when dir is actually a file")
	}
}

// --- Resume helper ---

func TestResume_loadsAndDelegates(t *testing.T) {
	store := durable.NewMemoryStore()
	state := mkState("resume-me")
	_ = store.SaveCheckpoint(context.Background(), state)

	var seen *agents.CheckpointState
	// Stub runner via a custom Middleware that captures opts.Resume and
	// returns immediately. We can't easily mock agents.Runner without a
	// chat.Client, so this test verifies durable.Resume's wiring rather
	// than the full loop.
	mw := func(_ agents.RunFunc) agents.RunFunc {
		return func(_ context.Context, _ *agents.Agent, opts agents.RunOptions) (*agents.RunResult, error) {
			seen = opts.Resume
			return &agents.RunResult{Output: "resumed"}, nil
		}
	}
	runner := &agents.Runner{Middleware: []agents.Middleware{mw}}

	res, err := durable.Resume(context.Background(), runner, store, "resume-me", &agents.Agent{Name: "Helper"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "resumed" {
		t.Errorf("output = %q", res.Output)
	}
	if seen == nil || seen.RunID != "resume-me" || seen.AgentName != "Helper" {
		t.Errorf("Resume option not threaded through: %+v", seen)
	}
}

func TestResume_loadMissingPropagates(t *testing.T) {
	store := durable.NewMemoryStore()
	runner := &agents.Runner{}
	_, err := durable.Resume(context.Background(), runner, store, "nope", &agents.Agent{Name: "x"})
	if !errors.Is(err, durable.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestResume_nilArgs(t *testing.T) {
	if _, err := durable.Resume(context.Background(), nil, nil, "x", nil); err == nil {
		t.Error("expected error for nil store/runner")
	}
}

// --- Store interface assertion ---

func TestMemoryStore_satisfiesStoreAndCheckpointer(t *testing.T) {
	var _ durable.Store = durable.NewMemoryStore()
	var _ agents.Checkpointer = durable.NewMemoryStore()
}

func TestFileStore_satisfiesStoreAndCheckpointer(t *testing.T) {
	s, err := durable.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var _ durable.Store = s
	var _ agents.Checkpointer = s
}
