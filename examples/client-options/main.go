// client-options demonstrates grok.WithLogger, grok.WithMaxRetries, and
// grok.WithConcurrency working together.
//
//   - WithLogger wires a slog.Logger into the transport layer so retries and
//     errors are visible in structured logs.
//   - WithMaxRetries controls how many times the transport retries 429/5xx
//     responses before giving up.
//   - WithConcurrency limits how many in-flight HTTP requests the client
//     makes at one time, preventing accidental rate-limit storms.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	client := grok.New(
		grok.WithAPIKey(os.Getenv("XAI_API_KEY")),
		grok.WithLogger(logger),
		grok.WithMaxRetries(3),
		grok.WithConcurrency(2), // at most 2 in-flight requests
	)

	prompts := []string{
		"Capital of France?",
		"Capital of Germany?",
		"Capital of Japan?",
		"Capital of Brazil?",
	}

	// Fire all 4 prompts concurrently; WithConcurrency(2) ensures only 2
	// run at the same time regardless of how many goroutines we spawn.
	var wg sync.WaitGroup
	ctx := context.Background()

	for i, p := range prompts {
		i, p := i, p
		wg.Add(1)
		go func() {
			defer wg.Done()
			comp, err := client.Chat.Create(ctx, &chat.CreateRequest{
				Model:    "grok-4-1-fast-non-reasoning",
				Messages: []chat.Message{{Role: "user", Content: p}},
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%d] error: %v\n", i+1, err)
				return
			}
			ans, _ := comp.Choices[0].Message.Content.(string)
			fmt.Printf("[%d] %s → %s\n", i+1, p, ans)
		}()
	}

	wg.Wait()
}
