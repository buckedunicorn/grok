package discord_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/discord"
	"github.com/buckedunicorn/grok/internal/transport"
)

// BenchmarkDiscordAgent_Handle_50turns_withSystem quantifies // every Handle currently builds a fresh [system, ...history] msgs slice
// before calling chat.RunAgent (which then copies it again). 50 sequential
// turns expose the per-turn allocation cost.
func BenchmarkDiscordAgent_Handle_50turns_withSystem(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{Message: chat.Message{Role: "assistant", Content: "ok"}, FinishReason: "stop"}},
		})
	}))
	defer srv.Close()
	tr := transport.NewInsecure("k", srv.URL, srv.Client())
	c := chat.NewClient(tr)

	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		a := discord.NewAgent(c, discord.AgentConfig{
			Model:  "m",
			System: "You are a helpful Discord bot persona prompt of moderate length.",
		})
		for range 50 {
			if _, err := a.Handle(ctx, "ch-1", "hello"); err != nil {
				b.Fatal(err)
			}
		}
	}
}
