package chat_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/buckedunicorn/grok/chat"
)

// regression: Create / Stream / CreateDeferred must not
// mutate the caller's *CreateRequest. Earlier versions of the SDK
// silently flipped req.Stream and req.Deferred, which surprised
// callers reusing the request struct across calls.

func TestCreate_doesNotMutateCallerRequest(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{Message: chat.Message{Role: "assistant", Content: "ok"}}},
		})
	})
	defer srv.Close()

	req := &chat.CreateRequest{
		Model:    "m",
		Messages: []chat.Message{{Role: "user", Content: "hi"}},
		Stream:   true, // staged for a Stream call later; Create must not flip this
	}
	if _, err := c.Create(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if !req.Stream {
		t.Errorf("Create flipped req.Stream from true to false")
	}
}

func TestCreateDeferred_doesNotMutateCallerRequest(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"request_id": "rid-1"})
	})
	defer srv.Close()

	req := &chat.CreateRequest{
		Model:    "m",
		Messages: []chat.Message{{Role: "user", Content: "hi"}},
		Stream:   true,
		Deferred: false,
	}
	if _, err := c.CreateDeferred(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if !req.Stream {
		t.Errorf("CreateDeferred mutated req.Stream")
	}
	if req.Deferred {
		t.Errorf("CreateDeferred mutated req.Deferred")
	}
}
