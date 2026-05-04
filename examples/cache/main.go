// cache demonstrates cache.Manager for tracking xAI prompt-cache hit rates.
// A long system prompt is reused across multiple turns; the x-grok-conv-id
// header keeps the KV cache warm so later turns pay fewer tokens.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/cache"
	"github.com/buckedunicorn/grok/chat"
)

func main() {
	mgr := cache.NewManager()

	// Pass the conversation ID so xAI can cache the shared prefix.
	client := grok.New(
		grok.WithAPIKey(os.Getenv("XAI_API_KEY")),
		grok.WithConvID(mgr.ConvID()),
	)

	// A long system prompt that benefits from caching.
	system := strings.Repeat("You are a helpful assistant. ", 50)

	questions := []string{
		"What is the capital of France?",
		"What is the capital of Japan?",
		"What is the capital of Brazil?",
	}

	ctx := context.Background()
	for i, q := range questions {
		comp, err := client.Chat.Create(ctx, &chat.CreateRequest{
			Model: "grok-4-1-fast-non-reasoning",
			Messages: []chat.Message{
				{Role: "system", Content: system},
				{Role: "user", Content: q},
			},
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		mgr.Record(comp.Usage)
		ans, _ := comp.Choices[0].Message.Content.(string)
		fmt.Printf("[%d] Q: %s\n    A: %s\n", i+1, q, ans)
	}

	s := mgr.Stats()
	fmt.Printf("\nconv_id: %s\n%s\n", mgr.ConvID(), s)
}
