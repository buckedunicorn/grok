package chat_test

import (
	"context"
	"os"
	"strings"
	"testing"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
)

func client(t *testing.T) *grok.Client {
	t.Helper()
	key := os.Getenv("XAI_API_KEY")
	if key == "" {
		t.Skip("XAI_API_KEY not set")
	}
	return grok.New(grok.WithAPIKey(key))
}

func TestIntegration_Create(t *testing.T) {
	c := client(t)
	resp, err := c.Chat.Create(context.Background(), &chat.CreateRequest{
		Model: "grok-3-mini-fast",
		Messages: []chat.Message{
			{Role: "user", Content: "Reply with one word: hello"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
}

func TestIntegration_Stream(t *testing.T) {
	c := client(t)
	stream, err := c.Chat.Stream(context.Background(), &chat.CreateRequest{
		Model: "grok-3-mini-fast",
		Messages: []chat.Message{
			{Role: "user", Content: "Reply with one word: hello"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	for {
		chunk, err := stream.Next()
		if err != nil {
			break
		}
		if len(chunk.Choices) > 0 {
			got.WriteString(chunk.Choices[0].Delta.Content)
		}
	}
	if got.Len() == 0 {
		t.Fatal("expected non-empty streamed content")
	}
}
