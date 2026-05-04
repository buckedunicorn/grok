package agents

import (
	"context"

	"github.com/buckedunicorn/grok/chat"
)

// SessionFromConversation adapts a chat.Conversation as a Session so callers
// can pass an existing stateful chat helper to Runner.Run.
//
// The adapter preserves chat.Conversation's existing behavior: Add appends
// items, Pop removes the most recent item, Clear calls Reset, Items returns
// the conversation's Messages snapshot. The system prompt configured on the
// Conversation is NOT exposed through Items, Runner re-injects the agent's
// own Instructions instead, which is the intended boundary.
func SessionFromConversation(id string, conv *chat.Conversation) Session {
	if id == "" {
		id = "conversation"
	}
	return &convSession{id: id, conv: conv}
}

type convSession struct {
	id   string
	conv *chat.Conversation
}

func (s *convSession) ID() string { return s.id }

func (s *convSession) Items(_ context.Context, limit int) ([]chat.Message, error) {
	all := s.conv.Messages()
	if limit <= 0 || limit >= len(all) {
		return all, nil
	}
	return all[len(all)-limit:], nil
}

// Snapshot satisfies SnapshotSession. Returns the
// underlying view of the Conversation history without copying. The
// agents.Runner sees this fast path automatically because it does
// type-asserts on SnapshotSession in sessionRead.
func (s *convSession) Snapshot(_ context.Context) ([]chat.Message, error) {
	return s.conv.MessagesView(), nil
}

func (s *convSession) Add(_ context.Context, items []chat.Message) error {
	s.conv.AppendMessages(items...)
	return nil
}

func (s *convSession) Pop(_ context.Context) (*chat.Message, error) {
	return s.conv.PopMessage(), nil
}

func (s *convSession) Clear(_ context.Context) error {
	s.conv.Reset()
	return nil
}
