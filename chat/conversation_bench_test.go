package chat_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/chat"
)

// BenchmarkConversation_Messages quantifies every call
// allocates a fresh slice of len(history). For a 50-message history
// this is one slice + 50 message-struct copies per call. SessionFrom
// Conversation invokes Messages once per Runner turn.
func BenchmarkConversation_Messages_50turns(b *testing.B) {
	cv := chat.NewConversation(nil, "m")
	for range 50 {
		cv.AppendMessages(chat.Message{Role: "user", Content: "hello"})
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = cv.Messages()
	}
}

// BenchmarkConversation_MessagesView confirms the fix:
// MessagesView does zero allocation regardless of history size.
// agents.SessionFromConversation routes through this for every Runner
// turn instead of paying a per-turn copy.
func BenchmarkConversation_MessagesView_50turns(b *testing.B) {
	cv := chat.NewConversation(nil, "m")
	for range 50 {
		cv.AppendMessages(chat.Message{Role: "user", Content: "hello"})
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = cv.MessagesView()
	}
}

// BenchmarkConversation_Send_50turns_withSystem confirms // the system message lives at messages[0] permanently so each Send
// appends in place rather than allocating a [system, ...history] copy.
//
// 50 sequential Sends against an in-memory mock chat server.
func BenchmarkConversation_Send_50turns_withSystem(b *testing.B) {
	srv, c := newChatClient(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{Message: chat.Message{Role: "assistant", Content: "ok"}}},
		})
	})
	defer srv.Close()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		cv := chat.NewConversation(c, "m", chat.WithSystem("You are concise."))
		for range 50 {
			if _, err := cv.Send(ctx, "ping"); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkConversation_Send_rolledBackLargeContent quantifies
// when Send fails after appending a large user
// message, the slot is truncated but the Message struct (Content any
// holding the large string) remains in the backing array. Repeated
// failures keep growing the array.
func BenchmarkConversation_Send_rolledBackLargeContent(b *testing.B) {
	srv, c := newChatClient(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"code":"x","error":"boom"}`, 500)
	})
	defer srv.Close()
	cv := chat.NewConversation(c, "m")
	largeContent := strings.Repeat("x", 64*1024) // 64 KiB
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = cv.Send(ctx, largeContent)
	}
}
