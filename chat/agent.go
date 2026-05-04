package chat

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrMaxTurnsExceeded is returned by RunAgent when the tool-call loop
// reaches the configured turn limit without the model finishing.
var ErrMaxTurnsExceeded = errors.New("grok: agent exceeded maximum tool-call turns")

// Handler is called when the model requests a tool invocation.
// argsJSON is the JSON-encoded arguments string from the model.
// The return value must be valid JSON (string, object, or array).
// On error, RunAgent sends the error message back to the model as tool output.
type Handler func(ctx context.Context, argsJSON string) (string, error)

// AgentOption configures a RunAgent call.
type AgentOption func(*agentConfig)

type agentConfig struct {
	maxTurns int
}

// WithMaxTurns limits how many tool-call rounds the agent may take.
// Defaults to 10. Pass 0 for no limit.
func WithMaxTurns(n int) AgentOption {
	return func(c *agentConfig) { c.maxTurns = n }
}

// RunAgent sends req and automatically handles tool calls using handlers,
// repeating until the model stops calling tools or maxTurns is reached.
// The caller's req.Messages slice is never mutated.
// Tool calls within a single turn are executed sequentially.
func (c *Client) RunAgent(
	ctx context.Context,
	req *CreateRequest,
	handlers map[string]Handler,
	opts ...AgentOption,
) (*Completion, error) {
	cfg := &agentConfig{maxTurns: 10}
	for _, o := range opts {
		o(cfg)
	}

	msgs := make([]Message, len(req.Messages))
	copy(msgs, req.Messages)

	for turn := 0; cfg.maxTurns == 0 || turn < cfg.maxTurns; turn++ {
		r := *req
		r.Messages = msgs
		r.Stream = false

		comp, err := c.Create(ctx, &r)
		if err != nil {
			return nil, err
		}
		if len(comp.Choices) == 0 || comp.Choices[0].FinishReason != "tool_calls" {
			return comp, nil
		}

		assistant := comp.Choices[0].Message
		// Guard against a server bug where finish_reason="tool_calls"
		// arrives with an empty tool_calls list. Without this, the
		// loop appends the assistant message, no tool results, and
		// re-sends. The model often replies the same way and we
		// burn turns until ctx cancels.
		if len(assistant.ToolCalls) == 0 {
			return nil, errors.New("grok: server returned finish_reason=tool_calls with no tool_calls")
		}
		msgs = append(msgs, assistant)

		for _, tc := range assistant.ToolCalls {
			result := callHandler(ctx, handlers, tc)
			msgs = append(msgs, Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result,
			})
		}
	}
	return nil, ErrMaxTurnsExceeded
}

func callHandler(ctx context.Context, handlers map[string]Handler, tc ToolCall) string {
	h, ok := handlers[tc.Function.Name]
	if !ok {
		return errorJSON("unknown tool " + tc.Function.Name)
	}
	result, err := h(ctx, tc.Function.Arguments)
	if err != nil {
		return errorJSON(err.Error())
	}
	return result
}

// errorJSON marshals msg as a JSON {"error":"..."} object using
// encoding/json so that any unicode or quote in msg is escaped
// correctly. Typed struct skips the map[string]string
// allocation per call.
func errorJSON(msg string) string {
	b, _ := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: msg})
	return string(b)
}
