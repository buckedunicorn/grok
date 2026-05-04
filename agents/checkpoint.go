package agents

import (
	"context"
	"time"

	"github.com/buckedunicorn/grok/chat"
)

// CheckpointState captures the state of an in-flight Run between turns.
// A Checkpointer persists this; a corresponding loader (e.g. durable.Store)
// reconstructs it for Resume.
//
// The shape is intentionally flat and JSON-friendly so file/KV/SQL storage
// backends can serialize it without custom encoders. Tool Handlers are NOT
// captured (they're Go functions); on Resume the caller passes the Agent
// with tools re-wired.
type CheckpointState struct {
	RunID      string         `json:"run_id"`
	NextTurn   int            `json:"next_turn"`  // turn index to execute next
	AgentName  string         `json:"agent_name"` // active agent at checkpoint time
	Messages   []chat.Message `json:"messages"`   // session history
	Trajectory Trajectory     `json:"trajectory"` // turns completed so far
	Usage      chat.Usage     `json:"usage"`      // cumulative
	SavedAt    time.Time      `json:"saved_at"`
}

// Checkpointer persists CheckpointState between turns. The Runner calls
// SaveCheckpoint after each turn that completes successfully (i.e. after
// the Trajectory.Turns slice has been appended to and before the next
// chat.Create call).
//
// A SaveCheckpoint error aborts the run and is returned wrapped from
// Runner.Run. Implementations that want to tolerate transient store
// failures should swallow them internally rather than expecting the
// Runner to retry.
type Checkpointer interface {
	SaveCheckpoint(ctx context.Context, state CheckpointState) error
}

// CheckpointerFunc adapts a plain function as a Checkpointer.
type CheckpointerFunc func(ctx context.Context, state CheckpointState) error

// SaveCheckpoint satisfies Checkpointer.
func (f CheckpointerFunc) SaveCheckpoint(ctx context.Context, state CheckpointState) error {
	return f(ctx, state)
}
