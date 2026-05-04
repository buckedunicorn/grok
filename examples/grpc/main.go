// grpc demonstrates making a chat completion via the xAI gRPC API.
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	grok "github.com/buckedunicorn/grok"
	xaiv1 "github.com/buckedunicorn/grok/grpc/gen/xai/api/v1"
)

func textMsg(role xaiv1.MessageRole, text string) *xaiv1.Message {
	return &xaiv1.Message{
		Role: role,
		Content: []*xaiv1.Content{
			{Content: &xaiv1.Content_Text{Text: text}},
		},
	}
}

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	gc, err := client.NewGRPC()
	if err != nil {
		fmt.Fprintln(os.Stderr, "grpc:", err)
		os.Exit(1)
	}
	defer gc.Close()

	ctx := gc.AuthContext(context.Background())

	// Non-streaming completion.
	resp, err := gc.Chat.GetCompletion(ctx, &xaiv1.GetCompletionsRequest{
		Model: "grok-4-1-fast-non-reasoning",
		Messages: []*xaiv1.Message{
			textMsg(xaiv1.MessageRole_ROLE_USER, "Say hello in one word."),
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "GetCompletion:", err)
		os.Exit(1)
	}
	if len(resp.GetOutputs()) > 0 {
		fmt.Println("response:", resp.GetOutputs()[0].GetMessage().GetContent())
	}

	// Streaming completion.
	fmt.Print("\nstreaming: ")
	stream, err := gc.Chat.GetCompletionChunk(ctx, &xaiv1.GetCompletionsRequest{
		Model: "grok-4-1-fast-non-reasoning",
		Messages: []*xaiv1.Message{
			textMsg(xaiv1.MessageRole_ROLE_USER, "Count to 5."),
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "GetCompletionChunk:", err)
		os.Exit(1)
	}
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "\nstream error:", err)
			os.Exit(1)
		}
		for _, out := range chunk.GetOutputs() {
			fmt.Print(out.GetDelta().GetContent())
		}
	}
	fmt.Println()
}
