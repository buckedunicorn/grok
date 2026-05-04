package chat_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/buckedunicorn/grok/chat"
)

// toolCompletion builds a Completion that requests a tool call.
func toolCompletion(toolCallID, name, args string) chat.Completion {
	return chat.Completion{
		Choices: []chat.Choice{{
			FinishReason: "tool_calls",
			Message: chat.Message{
				Role: "assistant",
				ToolCalls: []chat.ToolCall{{
					ID:   toolCallID,
					Type: "function",
					Function: chat.FunctionCallData{
						Name:      name,
						Arguments: args,
					},
				}},
			},
		}},
	}
}

// textCompletion builds a Completion with a plain text response.
func textCompletion(content string) chat.Completion {
	return chat.Completion{
		Choices: []chat.Choice{{
			FinishReason: "stop",
			Message:      chat.Message{Role: "assistant", Content: content},
		}},
	}
}

func TestAgent_noToolCalls(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(textCompletion("hello"))
	})
	defer srv.Close()

	comp, err := c.RunAgent(context.Background(),
		&chat.CreateRequest{Model: "m", Messages: []chat.Message{{Role: "user", Content: "hi"}}},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comp.Choices[0].Message.Content != "hello" {
		t.Errorf("unexpected content: %v", comp.Choices[0].Message.Content)
	}
}

func TestAgent_singleToolRound(t *testing.T) {
	calls := 0
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			json.NewEncoder(w).Encode(toolCompletion("tc1", "add", `{"a":1,"b":2}`))
		case 2:
			var req chat.CreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			// messages should be: user + assistant(tool_calls) + tool
			if len(req.Messages) != 3 {
				t.Errorf("turn 2: expected 3 messages, got %d", len(req.Messages))
			}
			last := req.Messages[len(req.Messages)-1]
			if last.Role != "tool" || last.Content != `3` {
				t.Errorf("unexpected tool result: %+v", last)
			}
			json.NewEncoder(w).Encode(textCompletion("the answer is 3"))
		}
	})
	defer srv.Close()

	handlers := map[string]chat.Handler{
		"add": func(ctx context.Context, args string) (string, error) {
			var p struct{ A, B int }
			json.Unmarshal([]byte(args), &p)
			return fmt.Sprint(p.A + p.B), nil
		},
	}
	comp, err := c.RunAgent(context.Background(),
		&chat.CreateRequest{Model: "m", Messages: []chat.Message{{Role: "user", Content: "what is 1+2?"}}},
		handlers,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comp.Choices[0].Message.Content != "the answer is 3" {
		t.Errorf("unexpected content: %v", comp.Choices[0].Message.Content)
	}
	if calls != 2 {
		t.Errorf("expected 2 API calls, got %d", calls)
	}
}

func TestAgent_multipleToolCallsPerTurn(t *testing.T) {
	calls := 0
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// Return two tool calls in one response.
			comp := chat.Completion{
				Choices: []chat.Choice{{
					FinishReason: "tool_calls",
					Message: chat.Message{
						Role: "assistant",
						ToolCalls: []chat.ToolCall{
							{ID: "tc1", Type: "function", Function: chat.FunctionCallData{Name: "ping", Arguments: `{}`}},
							{ID: "tc2", Type: "function", Function: chat.FunctionCallData{Name: "ping", Arguments: `{}`}},
						},
					},
				}},
			}
			json.NewEncoder(w).Encode(comp)
			return
		}
		var req chat.CreateRequest
		json.NewDecoder(r.Body).Decode(&req)
		// user + assistant(2 tool_calls) + 2 tool results = 4
		if len(req.Messages) != 4 {
			t.Errorf("expected 4 messages, got %d", len(req.Messages))
		}
		json.NewEncoder(w).Encode(textCompletion("done"))
	})
	defer srv.Close()

	var pingCount atomic.Int32
	handlers := map[string]chat.Handler{
		"ping": func(ctx context.Context, args string) (string, error) {
			pingCount.Add(1)
			return `"pong"`, nil
		},
	}
	_, err := c.RunAgent(context.Background(),
		&chat.CreateRequest{Model: "m", Messages: []chat.Message{{Role: "user", Content: "go"}}},
		handlers,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pingCount.Load() != 2 {
		t.Errorf("expected 2 ping calls, got %d", pingCount.Load())
	}
}

func TestAgent_unknownToolReturnsErrorJSON(t *testing.T) {
	calls := 0
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			json.NewEncoder(w).Encode(toolCompletion("tc1", "no_such_tool", `{}`))
			return
		}
		var req chat.CreateRequest
		json.NewDecoder(r.Body).Decode(&req)
		// tool result should contain "unknown tool"
		last := req.Messages[len(req.Messages)-1]
		if last.Role != "tool" {
			t.Errorf("expected tool message, got role=%s", last.Role)
		}
		json.NewEncoder(w).Encode(textCompletion("ok"))
	})
	defer srv.Close()

	_, err := c.RunAgent(context.Background(),
		&chat.CreateRequest{Model: "m", Messages: []chat.Message{{Role: "user", Content: "go"}}},
		map[string]chat.Handler{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgent_handlerErrorSentToModel(t *testing.T) {
	calls := 0
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			json.NewEncoder(w).Encode(toolCompletion("tc1", "fail", `{}`))
			return
		}
		var req chat.CreateRequest
		json.NewDecoder(r.Body).Decode(&req)
		last := req.Messages[len(req.Messages)-1]
		content, _ := last.Content.(string)
		if content == "" {
			t.Error("tool error content should not be empty")
		}
		json.NewEncoder(w).Encode(textCompletion("handled error"))
	})
	defer srv.Close()

	handlers := map[string]chat.Handler{
		"fail": func(ctx context.Context, args string) (string, error) {
			return "", errors.New("something broke")
		},
	}
	_, err := c.RunAgent(context.Background(),
		&chat.CreateRequest{Model: "m", Messages: []chat.Message{{Role: "user", Content: "go"}}},
		handlers,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgent_maxTurnsExceeded(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(toolCompletion("tc1", "loop", `{}`))
	})
	defer srv.Close()

	handlers := map[string]chat.Handler{
		"loop": func(ctx context.Context, args string) (string, error) { return `"ok"`, nil },
	}
	_, err := c.RunAgent(context.Background(),
		&chat.CreateRequest{Model: "m", Messages: []chat.Message{{Role: "user", Content: "go"}}},
		handlers,
		chat.WithMaxTurns(3),
	)
	if !errors.Is(err, chat.ErrMaxTurnsExceeded) {
		t.Errorf("expected ErrMaxTurnsExceeded, got: %v", err)
	}
}

func TestAgent_doesNotMutateCallerMessages(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(textCompletion("ok"))
	})
	defer srv.Close()

	original := []chat.Message{{Role: "user", Content: "hi"}}
	req := &chat.CreateRequest{Model: "m", Messages: original}

	c.RunAgent(context.Background(), req, nil) //nolint:errcheck

	if len(req.Messages) != 1 {
		t.Errorf("RunAgent mutated req.Messages (len=%d)", len(req.Messages))
	}
}

func TestAgent_contextCancelled(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"code":"internal","error":"server error"}`))
	})
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.RunAgent(ctx,
		&chat.CreateRequest{Model: "m", Messages: []chat.Message{{Role: "user", Content: "hi"}}},
		nil,
	)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}
