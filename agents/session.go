package agents

import (
	"context"
	"sync"

	"github.com/buckedunicorn/grok/chat"
)

// Session persists conversation history across turns and across Runs.
// Implementations must be safe for concurrent use by a single Runner; sharing
// a Session across concurrent Runs is undefined.
type Session interface {
	// ID returns a stable identifier for the session.
	ID() string
	// Items returns up to limit most-recent messages in chronological order.
	// Pass 0 to retrieve all items.
	Items(ctx context.Context, limit int) ([]chat.Message, error)
	// Add appends items to the session history.
	Add(ctx context.Context, items []chat.Message) error
	// Pop removes and returns the most recent item, or nil if empty.
	Pop(ctx context.Context) (*chat.Message, error)
	// Clear empties the session.
	Clear(ctx context.Context) error
}

// SnapshotSession is an optional fast-path interface that a Session
// implementation may satisfy to avoid the per-turn defensive copy that
// Items() performs. Snapshot returns the underlying slice; the caller
// MUST NOT mutate the returned slice. The Runner uses this when the
// concrete Session supports it.
type SnapshotSession interface {
	Snapshot(ctx context.Context) ([]chat.Message, error)
}

// InMemorySession is a Session implementation backed by an in-process slice.
// Safe for concurrent use within a single Runner.
type InMemorySession struct {
	id    string
	mu    sync.Mutex
	items []chat.Message
}

// NewInMemorySession creates a new in-memory Session with the given id.
// If id is empty, "in-memory" is used.
func NewInMemorySession(id string) *InMemorySession {
	if id == "" {
		id = "in-memory"
	}
	return &InMemorySession{id: id}
}

func (s *InMemorySession) ID() string { return s.id }

func (s *InMemorySession) Items(_ context.Context, limit int) ([]chat.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit >= len(s.items) {
		out := make([]chat.Message, len(s.items))
		copy(out, s.items)
		return out, nil
	}
	start := len(s.items) - limit
	out := make([]chat.Message, limit)
	copy(out, s.items[start:])
	return out, nil
}

// Snapshot satisfies SnapshotSession. Returns the underlying slice for
// read-only use; the caller MUST NOT mutate it. Permits the Runner to
// skip the per-turn defensive copy that Items() performs
// .
func (s *InMemorySession) Snapshot(_ context.Context) ([]chat.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items, nil
}

func (s *InMemorySession) Add(_ context.Context, items []chat.Message) error {
	s.mu.Lock()
	s.items = append(s.items, items...)
	s.mu.Unlock()
	return nil
}

func (s *InMemorySession) Pop(_ context.Context) (*chat.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) == 0 {
		return nil, nil
	}
	last := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return &last, nil
}

func (s *InMemorySession) Clear(_ context.Context) error {
	s.mu.Lock()
	s.items = nil
	s.mu.Unlock()
	return nil
}
