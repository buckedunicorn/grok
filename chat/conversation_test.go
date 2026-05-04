package chat_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/internal/transport"
)

func newChatClient(handler http.HandlerFunc) (*httptest.Server, *chat.Client) {
	srv := httptest.NewServer(handler)
	t := transport.NewInsecure("test-key", srv.URL, srv.Client())
	return srv, chat.NewClient(t)
}

func TestConversation_Send_singleTurn(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		var req chat.CreateRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
			t.Errorf("unexpected messages: %+v", req.Messages)
		}
		json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{
				{Message: chat.Message{Role: "assistant", Content: "world"}},
			},
		})
	})
	defer srv.Close()

	cv := chat.NewConversation(c, "grok-test")
	comp, err := cv.Send(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comp.Choices) == 0 || comp.Choices[0].Message.Content != "world" {
		t.Errorf("unexpected completion: %+v", comp)
	}
	msgs := cv.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages in history, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Errorf("unexpected roles: %s %s", msgs[0].Role, msgs[1].Role)
	}
}

func TestConversation_Send_multiTurn_accumulatesHistory(t *testing.T) {
	turn := 0
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		turn++
		var req chat.CreateRequest
		json.NewDecoder(r.Body).Decode(&req)
		// turn 1: 1 msg; turn 2: 3 msgs (user+assistant+user)
		if turn == 2 && len(req.Messages) != 3 {
			t.Errorf("turn 2: expected 3 messages, got %d", len(req.Messages))
		}
		json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{
				{Message: chat.Message{Role: "assistant", Content: "reply"}},
			},
		})
	})
	defer srv.Close()

	cv := chat.NewConversation(c, "grok-test")
	cv.Send(context.Background(), "first")  //nolint:errcheck
	cv.Send(context.Background(), "second") //nolint:errcheck

	if len(cv.Messages()) != 4 {
		t.Errorf("expected 4 messages after 2 turns, got %d", len(cv.Messages()))
	}
}

func TestConversation_Send_withSystem(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		var req chat.CreateRequest
		json.NewDecoder(r.Body).Decode(&req)
		// system prompt should be first in request messages
		if len(req.Messages) < 1 || req.Messages[0].Role != "system" {
			t.Errorf("expected system message first, got: %+v", req.Messages)
		}
		// system prompt must not leak into stored history
		json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{
				{Message: chat.Message{Role: "assistant", Content: "ok"}},
			},
		})
	})
	defer srv.Close()

	cv := chat.NewConversation(c, "grok-test", chat.WithSystem("you are helpful"))
	cv.Send(context.Background(), "hi") //nolint:errcheck

	msgs := cv.Messages()
	for _, m := range msgs {
		if m.Role == "system" {
			t.Error("system message must not appear in stored history")
		}
	}
}

func TestConversation_Send_errorRollsBack(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"code":"internal","error":"server error"}`))
	})
	defer srv.Close()

	cv := chat.NewConversation(c, "grok-test")
	_, err := cv.Send(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(cv.Messages()) != 0 {
		t.Errorf("history should be empty after rollback, got %d messages", len(cv.Messages()))
	}
}

func TestConversation_Reset(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{
				{Message: chat.Message{Role: "assistant", Content: "ok"}},
			},
		})
	})
	defer srv.Close()

	cv := chat.NewConversation(c, "grok-test")
	cv.Send(context.Background(), "hello") //nolint:errcheck
	if len(cv.Messages()) == 0 {
		t.Fatal("expected messages before reset")
	}
	cv.Reset()
	if len(cv.Messages()) != 0 {
		t.Errorf("expected empty history after reset, got %d", len(cv.Messages()))
	}
}

func TestConversation_SendParts_multipart(t *testing.T) {
	var seenParts []chat.ContentPart
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		var req chat.CreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		// Last message must be the user's multipart content. The JSON
		// decode round-trips ContentPart through Message.Content (any),
		// so we re-marshal/decode to assert structure.
		if len(req.Messages) == 0 {
			t.Fatal("no messages in request")
		}
		raw, _ := json.Marshal(req.Messages[len(req.Messages)-1].Content)
		_ = json.Unmarshal(raw, &seenParts)
		_ = json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{Message: chat.Message{Role: "assistant", Content: "got it"}}},
		})
	})
	defer srv.Close()

	cv := chat.NewConversation(c, "grok-test")
	parts := []chat.ContentPart{
		{Type: "text", Text: "what's in this picture?"},
		{Type: "image_url", ImageURL: &chat.ImageURL{URL: "https://example.com/cat.png"}},
	}
	comp, err := cv.SendParts(context.Background(), parts)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := comp.Choices[0].Message.Content.(string); got != "got it" {
		t.Errorf("reply = %q", got)
	}
	if len(seenParts) != 2 {
		t.Fatalf("server saw %d parts, want 2", len(seenParts))
	}
	if seenParts[0].Type != "text" || seenParts[0].Text == "" {
		t.Errorf("first part wrong: %+v", seenParts[0])
	}
	if seenParts[1].Type != "image_url" || seenParts[1].ImageURL == nil ||
		seenParts[1].ImageURL.URL != "https://example.com/cat.png" {
		t.Errorf("image part wrong: %+v", seenParts[1])
	}
	// History should now contain user(parts) + assistant("got it").
	hist := cv.Messages()
	if len(hist) != 2 {
		t.Fatalf("history = %d, want 2", len(hist))
	}
	if _, ok := hist[0].Content.([]chat.ContentPart); !ok {
		t.Errorf("user message in history should retain []ContentPart, got %T", hist[0].Content)
	}
}

func TestConversation_SendParts_errorRollsBack(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"code":"internal","error":"boom"}`))
	})
	defer srv.Close()

	cv := chat.NewConversation(c, "grok-test")
	_, err := cv.SendParts(context.Background(), []chat.ContentPart{
		{Type: "text", Text: "hi"},
	})
	if err == nil {
		t.Fatal("expected error from 500")
	}
	if got := len(cv.Messages()); got != 0 {
		t.Errorf("history should roll back, got %d messages", got)
	}
}
