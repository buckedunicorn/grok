package agents

import (
	"context"

	"github.com/buckedunicorn/grok/chat"
)

// RunHooks is a set of optional callbacks invoked at key points in the Runner
// loop. Each callback may be nil. Hooks must not block on long-running work;
// for streaming export to OTel/slog, prefer a Middleware that processes the
// final Trajectory after Run returns.
//
// RedactToolArgs and RedactToolResult, when non-nil, are called before
// the corresponding string is recorded into the Trajectory. Use them to
// strip secrets from arguments or results that flow through tool
// boundaries. The returned string replaces the
// original in ToolCallTrace; the original is still passed to the
// handler unmodified.
type RunHooks struct {
	OnTurnStart      func(ctx context.Context, turn int, agent *Agent, input []chat.Message)
	OnToolCall       func(ctx context.Context, turn int, name, argsJSON string)
	OnToolResult     func(ctx context.Context, turn int, name, result string, err error)
	OnHandoff        func(ctx context.Context, from, to *Agent, reason string)
	OnTurnEnd        func(ctx context.Context, turn int, comp *chat.Completion)
	RedactToolArgs   func(ctx context.Context, name, argsJSON string) string
	RedactToolResult func(ctx context.Context, name, result string) string
}
