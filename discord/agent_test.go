package discord_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/discord"
	"github.com/buckedunicorn/grok/internal/transport"
)

// newAgent wires an in-memory chat server into a discord.Agent for
// table-driven tests. handler runs on every chat completion request;
// it owns the mock response.
func newAgent(handler http.HandlerFunc, cfg discord.AgentConfig) (*httptest.Server, *discord.Agent) {
	srv := httptest.NewServer(handler)
	tr := transport.NewInsecure("test-key", srv.URL, srv.Client())
	return srv, discord.NewAgent(chat.NewClient(tr), cfg)
}

func okReply(reply string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{
				{Message: chat.Message{Role: "assistant", Content: reply}, FinishReason: "stop"},
			},
		})
	}
}

func TestAgent_Handle_basic(t *testing.T) {
	srv, a := newAgent(okReply("hi back"), discord.AgentConfig{Model: "m", System: "be brief"})
	defer srv.Close()

	reply, err := a.Handle(context.Background(), "ch1", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if reply != "hi back" {
		t.Errorf("reply = %q, want %q", reply, "hi back")
	}
}

func TestAgent_separateHistoriesPerChannel(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	srv, a := newAgent(func(w http.ResponseWriter, r *http.Request) {
		var cr chat.CreateRequest
		_ = json.NewDecoder(r.Body).Decode(&cr)
		// Last user message in this turn.
		mu.Lock()
		for _, m := range cr.Messages {
			if m.Role == "user" {
				if s, ok := m.Content.(string); ok {
					seen[s]++
				}
			}
		}
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{Message: chat.Message{Role: "assistant", Content: "ack"}, FinishReason: "stop"}},
		})
	}, discord.AgentConfig{Model: "m"})
	defer srv.Close()

	_, _ = a.Handle(context.Background(), "ch1", "msg-a")
	_, _ = a.Handle(context.Background(), "ch2", "msg-b")
	_, _ = a.Handle(context.Background(), "ch1", "msg-a2")

	// ch1's second turn should include "msg-a" + "msg-a2"; ch2 only "msg-b".
	mu.Lock()
	defer mu.Unlock()
	if seen["msg-a"] != 2 { // first turn + replayed in second turn's history
		t.Errorf("msg-a seen %d times, want 2", seen["msg-a"])
	}
	if seen["msg-a2"] != 1 {
		t.Errorf("msg-a2 seen %d times, want 1", seen["msg-a2"])
	}
	if seen["msg-b"] != 1 {
		t.Errorf("msg-b seen %d times, want 1", seen["msg-b"])
	}
}

func TestAgent_ToolCallLoop(t *testing.T) {
	// Server responds with a tool_call on first request, then "all
	// done" after the tool result is appended. Confirms Agent uses
	// chat.RunAgent under the hood.
	turn := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		turn++
		t := turn
		mu.Unlock()
		var cr chat.CreateRequest
		_ = json.NewDecoder(r.Body).Decode(&cr)
		if t == 1 {
			// First call: ask the model to invoke "ping".
			_ = json.NewEncoder(w).Encode(chat.Completion{
				Choices: []chat.Choice{{
					FinishReason: "tool_calls",
					Message: chat.Message{
						Role: "assistant",
						ToolCalls: []chat.ToolCall{{
							ID:       "tc-1",
							Type:     "function",
							Function: chat.FunctionCallData{Name: "ping", Arguments: `{"x":1}`},
						}},
					},
				}},
			})
			return
		}
		// Second call: stop loop with a final answer.
		_ = json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{
				FinishReason: "stop",
				Message:      chat.Message{Role: "assistant", Content: "pong: ok"},
			}},
		})
	}))
	defer srv.Close()

	pingHits := 0
	tr := transport.NewInsecure("test-key", srv.URL, srv.Client())
	a := discord.NewAgent(chat.NewClient(tr), discord.AgentConfig{
		Model: "m",
		Tools: []chat.Tool{
			{Type: "function", Function: chat.FunctionDef{Name: "ping"}},
		},
		Handlers: map[string]chat.Handler{
			"ping": func(_ context.Context, _ string) (string, error) {
				pingHits++
				return `{"ok":true}`, nil
			},
		},
	})

	reply, err := a.Handle(context.Background(), "ch1", "go")
	if err != nil {
		t.Fatal(err)
	}
	if reply != "pong: ok" {
		t.Errorf("reply = %q", reply)
	}
	if pingHits != 1 {
		t.Errorf("ping handler hit %d times, want 1", pingHits)
	}
	if turn != 2 {
		t.Errorf("server saw %d chat completions, want 2 (request + response-to-tool-result)", turn)
	}
}

func TestAgent_HandleParts_forwardsImage(t *testing.T) {
	var seenParts []chat.ContentPart
	srv, a := newAgent(func(w http.ResponseWriter, r *http.Request) {
		var cr chat.CreateRequest
		_ = json.NewDecoder(r.Body).Decode(&cr)
		if n := len(cr.Messages); n > 0 {
			raw, _ := json.Marshal(cr.Messages[n-1].Content)
			_ = json.Unmarshal(raw, &seenParts)
		}
		_ = json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{Message: chat.Message{Role: "assistant", Content: "i see it"}, FinishReason: "stop"}},
		})
	}, discord.AgentConfig{Model: "m"})
	defer srv.Close()

	parts := []chat.ContentPart{
		{Type: "text", Text: "what is this?"},
		{Type: "image_url", ImageURL: &chat.ImageURL{URL: "https://example.com/cat.png", Detail: "auto"}},
	}
	reply, err := a.HandleParts(context.Background(), "ch1", parts)
	if err != nil {
		t.Fatal(err)
	}
	if reply != "i see it" {
		t.Errorf("reply = %q", reply)
	}
	if len(seenParts) != 2 || seenParts[1].ImageURL == nil ||
		seenParts[1].ImageURL.URL != "https://example.com/cat.png" {
		t.Errorf("image part not forwarded: %+v", seenParts)
	}
}

func TestAgent_Reset_clearsHistory(t *testing.T) {
	srv, a := newAgent(okReply("ack"), discord.AgentConfig{Model: "m"})
	defer srv.Close()

	_, _ = a.Handle(context.Background(), "ch1", "first")
	if got := len(a.Messages("ch1")); got == 0 {
		t.Errorf("expected history before reset, got 0")
	}
	a.Reset("ch1")
	if got := len(a.Messages("ch1")); got != 0 {
		t.Errorf("history after Reset = %d, want 0", got)
	}
}

// regression: Messages must not insert empty entries
// for unknown channel IDs.
func TestAgent_Messages_doesNotCreateEntryForUnknownChannel(t *testing.T) {
	srv, a := newAgent(okReply("ok"), discord.AgentConfig{Model: "m"})
	defer srv.Close()

	got := a.Messages("never-seen")
	if got != nil {
		t.Errorf("Messages(unknown) = %v, want nil", got)
	}

	// Resetting a channel that was created by polling should not panic
	// and should leave the agent in a clean state. (Pre-fix Reset on a
	// "never-seen" channel was a no-op only because Messages had
	// already created the entry; verify no entry exists now.)
	a.Reset("never-seen") // no-op
}

func TestAgent_ChannelContext_threadsToHandler(t *testing.T) {
	// Agent.Handle should put channelID onto ctx so a tool handler can
	// retrieve it via discord.ChannelFromContext.
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cr chat.CreateRequest
		_ = json.NewDecoder(r.Body).Decode(&cr)
		// First turn: emit a tool call. Handler will read ctx.
		hasToolMsg := false
		for _, m := range cr.Messages {
			if m.Role == "tool" {
				hasToolMsg = true
				break
			}
		}
		if !hasToolMsg {
			_ = json.NewEncoder(w).Encode(chat.Completion{
				Choices: []chat.Choice{{
					FinishReason: "tool_calls",
					Message: chat.Message{
						Role: "assistant",
						ToolCalls: []chat.ToolCall{{
							ID:       "tc-1",
							Type:     "function",
							Function: chat.FunctionCallData{Name: "probe", Arguments: `{}`},
						}},
					},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{
				FinishReason: "stop",
				Message:      chat.Message{Role: "assistant", Content: "done"},
			}},
		})
	}))
	defer srv.Close()

	tr := transport.NewInsecure("test-key", srv.URL, srv.Client())
	a := discord.NewAgent(chat.NewClient(tr), discord.AgentConfig{
		Model: "m",
		Tools: []chat.Tool{{Type: "function", Function: chat.FunctionDef{Name: "probe"}}},
		Handlers: map[string]chat.Handler{
			"probe": func(ctx context.Context, _ string) (string, error) {
				seen = discord.ChannelFromContext(ctx)
				return `{}`, nil
			},
		},
	})

	if _, err := a.Handle(context.Background(), "ch-42", "go"); err != nil {
		t.Fatal(err)
	}
	if seen != "ch-42" {
		t.Errorf("handler saw channelID = %q, want %q", seen, "ch-42")
	}
}
