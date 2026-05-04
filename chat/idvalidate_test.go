package chat_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/internal/transport"
)

// regression: chat.GetDeferred validates the requestID
// before issuing any HTTP request. A malformed ID (path traversal,
// control bytes, query-injection) is rejected client-side.
func TestGetDeferred_rejectsMalformedRequestID(t *testing.T) {
	tr := transport.NewInsecure("k", "http://unused.invalid", nil)
	c := chat.NewClient(tr)

	for _, id := range []string{
		"",
		"../leak",
		"abc/def",
		"abc?injected=1",
		"abc#fragment",
		"abc%252e%252e",
		"valid\nbreak",
	} {
		_, err := c.GetDeferred(context.Background(), id)
		if err == nil {
			t.Errorf("expected error from GetDeferred(%q), got nil", id)
		}
	}
}

// regression: RunAgent must surface a clear error rather
// than infinite-loop when the server returns finish_reason=tool_calls
// with no tool_calls field.
func TestRunAgent_rejectsToolCallsFinishWithEmptyToolCalls(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, _ *http.Request) {
		// Returns finish_reason=tool_calls but with no tool_calls.
		// Pre-fix this would loop forever when WithMaxTurns(0).
		_ = json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{
				FinishReason: "tool_calls",
				Message:      chat.Message{Role: "assistant"},
			}},
		})
	})
	defer srv.Close()

	_, err := c.RunAgent(context.Background(),
		&chat.CreateRequest{
			Model:    "m",
			Messages: []chat.Message{{Role: "user", Content: "hi"}},
		},
		map[string]chat.Handler{},
		chat.WithMaxTurns(0), // 0 = unlimited; the bug used to wedge here
	)
	if err == nil {
		t.Fatal("expected error from RunAgent on empty tool_calls, got nil")
	}
	if !strings.Contains(err.Error(), "tool_calls") {
		t.Errorf("error %q should mention tool_calls", err)
	}
}
