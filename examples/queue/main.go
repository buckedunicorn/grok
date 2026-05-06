// queue demonstrates the rate-limiting request queue.
// Five prompts are sent to the API at most 2 requests per second,
// preventing 429 rate-limit errors when making many calls in a loop.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/queue"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	// Allow at most 2 requests per second.
	q := queue.New(2.0)

	prompts := []string{
		"Capital of France?",
		"Capital of Germany?",
		"Capital of Japan?",
		"Capital of Brazil?",
		"Capital of Australia?",
	}

	start := time.Now()
	for i, prompt := range prompts {
		i, prompt := i, prompt
		err := q.Submit(context.Background(), func() error {
			comp, err := client.Chat.Create(context.Background(), &chat.CreateRequest{
				Model:               "grok-4-1-fast-non-reasoning",
				Messages:            []chat.Message{{Role: "user", Content: prompt}},
				MaxCompletionTokens: intPtr(10),
			})
			if err != nil {
				return err
			}
			ans, _ := comp.Choices[0].Message.Content.(string)
			fmt.Printf("[%d] %s → %s\n", i+1, prompt, ans)
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	fmt.Printf("\n5 requests in %v (rate-limited to 2 rps)\n", time.Since(start).Round(time.Millisecond))
}

func intPtr(n int) *int { return &n }
