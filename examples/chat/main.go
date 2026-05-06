// chat demonstrates a single non-streaming chat completion.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	resp, err := client.Chat.Create(context.Background(), &chat.CreateRequest{
		Model: "grok-4-1-fast-non-reasoning",
		Messages: []chat.Message{
			{Role: "system", Content: "You are a concise assistant."},
			{Role: "user", Content: "What is the capital of France?"},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(resp.Choices[0].Message.Content)
	fmt.Printf("\ntokens: %d prompt / %d completion\n",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
}
