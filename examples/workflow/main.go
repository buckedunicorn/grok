// workflow demonstrates composing Grok calls as workflow Steps.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/workflow"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	// Step: summarise input text.
	summarise := func(ctx context.Context, text string) (string, error) {
		comp, err := client.Chat.Create(ctx, &chat.CreateRequest{
			Model: "grok-4-1-fast-reasoning",
			Messages: []chat.Message{
				{Role: "system", Content: "Summarise the following in one sentence."},
				{Role: "user", Content: text},
			},
		})
		if err != nil {
			return "", err
		}
		s, _ := comp.Choices[0].Message.Content.(string)
		return s, nil
	}

	// Step: translate to French.
	translate := func(ctx context.Context, text string) (string, error) {
		comp, err := client.Chat.Create(ctx, &chat.CreateRequest{
			Model: "grok-4-1-fast-reasoning",
			Messages: []chat.Message{
				{Role: "system", Content: "Translate the following text to French. Output only the translation."},
				{Role: "user", Content: text},
			},
		})
		if err != nil {
			return "", err
		}
		s, _ := comp.Choices[0].Message.Content.(string)
		return s, nil
	}

	input := "Go is an open-source programming language supported by Google. " +
		"It is expressive, concise, clean, and efficient."

	// Chain: summarise → translate → uppercase
	result, err := workflow.Chain(context.Background(), input,
		summarise,
		translate,
		workflow.Transform(strings.ToUpper),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(result)
}
