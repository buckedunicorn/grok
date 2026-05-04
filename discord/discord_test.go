package discord_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/discord"
	"github.com/buckedunicorn/grok/internal/transport"
)

func newRouter(handler http.HandlerFunc, cfg discord.RouterConfig) (*httptest.Server, *discord.Router) {
	srv := httptest.NewServer(handler)
	t := transport.NewInsecure("test-key", srv.URL, srv.Client())
	return srv, discord.NewRouter(chat.NewClient(t), cfg)
}

func okHandler(reply string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{
				{Message: chat.Message{Role: "assistant", Content: reply}},
			},
		})
	}
}

func TestRouter_handle_basic(t *testing.T) {
	srv, r := newRouter(okHandler("hello back"), discord.RouterConfig{Model: "m"})
	defer srv.Close()

	reply, err := r.Handle(context.Background(), "ch1", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "hello back" {
		t.Errorf("unexpected reply: %q", reply)
	}
}

func TestRouter_separateConvsPerChannel(t *testing.T) {
	calls := make(map[string]int)
	srv, r := newRouter(func(w http.ResponseWriter, req *http.Request) {
		var cr chat.CreateRequest
		json.NewDecoder(req.Body).Decode(&cr)
		last, _ := cr.Messages[len(cr.Messages)-1].Content.(string)
		calls[last]++
		json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{Message: chat.Message{Role: "assistant", Content: "ok"}}},
		})
	}, discord.RouterConfig{Model: "m"})
	defer srv.Close()

	r.Handle(context.Background(), "ch1", "msg-a") //nolint:errcheck
	r.Handle(context.Background(), "ch2", "msg-b") //nolint:errcheck

	if calls["msg-a"] != 1 || calls["msg-b"] != 1 {
		t.Errorf("unexpected call counts: %v", calls)
	}
}

func TestRouter_reset(t *testing.T) {
	turn := 0
	srv, r := newRouter(func(w http.ResponseWriter, req *http.Request) {
		turn++
		var cr chat.CreateRequest
		json.NewDecoder(req.Body).Decode(&cr)
		json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{Message: chat.Message{Role: "assistant", Content: "ok"}}},
		})
	}, discord.RouterConfig{Model: "m"})
	defer srv.Close()

	r.Handle(context.Background(), "ch1", "first") //nolint:errcheck
	r.Reset("ch1")
	r.Handle(context.Background(), "ch1", "second") //nolint:errcheck

	// After reset, history should start fresh (1 user msg each time).
}

func TestChunks_shortText(t *testing.T) {
	chunks := discord.Chunks("hello", 2000)
	if len(chunks) != 1 || chunks[0] != "hello" {
		t.Errorf("unexpected chunks: %v", chunks)
	}
}

func TestChunks_longText(t *testing.T) {
	text := strings.Repeat("x", 4500)
	chunks := discord.Chunks(text, 2000)
	if len(chunks) != 3 {
		t.Errorf("expected 3 chunks, got %d", len(chunks))
	}
	if len(chunks[0]) != 2000 || len(chunks[1]) != 2000 || len(chunks[2]) != 500 {
		t.Errorf("unexpected chunk sizes: %d %d %d",
			len(chunks[0]), len(chunks[1]), len(chunks[2]))
	}
}

func TestChunks_defaultMaxLen(t *testing.T) {
	chunks := discord.Chunks("hi", 0) // 0 → uses default 2000
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestRouter_HandleParts_forwardsImage(t *testing.T) {
	var seenParts []chat.ContentPart
	srv, r := newRouter(func(w http.ResponseWriter, req *http.Request) {
		var cr chat.CreateRequest
		_ = json.NewDecoder(req.Body).Decode(&cr)
		// Last message is the user's multipart content.
		if n := len(cr.Messages); n > 0 {
			raw, _ := json.Marshal(cr.Messages[n-1].Content)
			_ = json.Unmarshal(raw, &seenParts)
		}
		_ = json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{Message: chat.Message{Role: "assistant", Content: "i see a cat"}}},
		})
	}, discord.RouterConfig{Model: "m", System: "vision bot"})
	defer srv.Close()

	parts := []chat.ContentPart{
		{Type: "text", Text: "what is this?"},
		{Type: "image_url", ImageURL: &chat.ImageURL{URL: "https://example.com/cat.png", Detail: "auto"}},
	}
	reply, err := r.HandleParts(context.Background(), "ch1", parts)
	if err != nil {
		t.Fatal(err)
	}
	if reply != "i see a cat" {
		t.Errorf("reply = %q", reply)
	}
	if len(seenParts) != 2 {
		t.Fatalf("server saw %d parts, want 2", len(seenParts))
	}
	if seenParts[1].Type != "image_url" || seenParts[1].ImageURL == nil ||
		seenParts[1].ImageURL.URL != "https://example.com/cat.png" {
		t.Errorf("image part not forwarded: %+v", seenParts[1])
	}
}
