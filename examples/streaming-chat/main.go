// streaming-chat demonstrates token-by-token streaming using SSE.
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	stream, err := client.Chat.Stream(context.Background(), &chat.CreateRequest{
		Model: "grok-4-1-fast-non-reasoning",
		Messages: []chat.Message{
			{Role: "user", Content: "Count from 1 to 10, one number per line."},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer stream.Close()

	for {
		chunk, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if len(chunk.Choices) > 0 {
			fmt.Print(chunk.Choices[0].Delta.Content)
		}
	}
	fmt.Println()
}
