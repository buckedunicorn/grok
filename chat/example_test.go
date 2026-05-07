package chat_test

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
)

func ExampleClient_Create() {
	client := grok.New()
	comp, err := client.Chat.Create(context.Background(), &chat.CreateRequest{
		Model: "grok-3-mini-fast",
		Messages: []chat.Message{
			{Role: "user", Content: "What is the speed of light?"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(comp.Choices[0].Message.Content)
}

func ExampleClient_Stream() {
	client := grok.New()
	stream, err := client.Chat.Stream(context.Background(), &chat.CreateRequest{
		Model: "grok-3-mini-fast",
		Messages: []chat.Message{
			{Role: "user", Content: "Write a haiku about Go."},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()

	for {
		chunk, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		if len(chunk.Choices) > 0 {
			fmt.Print(chunk.Choices[0].Delta.Content)
		}
	}
	fmt.Println()
}

func ExampleConversation_Send() {
	client := grok.New()
	conv := chat.NewConversation(client.Chat, "grok-3-mini-fast",
		chat.WithSystem("You are a concise assistant."),
	)

	reply, err := conv.Send(context.Background(), "What is Go?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(reply.Choices[0].Message.Content)

	// History is carried forward automatically.
	reply2, err := conv.Send(context.Background(), "Who designed it?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(reply2.Choices[0].Message.Content)
}

func ExampleDecode() {
	type Point struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}

	client := grok.New()
	comp, err := client.Chat.Create(context.Background(), &chat.CreateRequest{
		Model: "grok-3-mini-fast",
		Messages: []chat.Message{
			{Role: "user", Content: `Return exactly: {"x": 1.0, "y": 2.0}`},
		},
		ResponseFormat: &chat.ResponseFormat{Type: "json_object"},
	})
	if err != nil {
		log.Fatal(err)
	}

	p, err := chat.Decode[Point](comp)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("x=%.1f y=%.1f\n", p.X, p.Y)
}

func ExampleClient_RunAgent() {
	weatherTool := chat.Tool{
		Type: "function",
		Function: chat.FunctionDef{
			Name:        "get_weather",
			Description: "Returns the current weather for a city.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
				"required": []string{"city"},
			},
		},
	}

	handlers := map[string]chat.Handler{
		"get_weather": func(_ context.Context, _ string) (string, error) {
			return `{"temp": "22°C", "condition": "sunny"}`, nil
		},
	}

	client := grok.New()
	comp, err := client.Chat.RunAgent(context.Background(),
		&chat.CreateRequest{
			Model: "grok-3-mini-fast",
			Messages: []chat.Message{
				{Role: "user", Content: "What is the weather in Paris?"},
			},
			Tools: []chat.Tool{weatherTool},
		},
		handlers,
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(comp.Choices[0].Message.Content)
}
