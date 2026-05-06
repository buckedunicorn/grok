// tool-use demonstrates the RunAgent loop: the model is given two tools
// (get_weather and add_numbers) and RunAgent handles all tool calls automatically
// until the model returns a final text answer.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	tools := []chat.Tool{
		{
			Type: "function",
			Function: chat.FunctionDef{
				Name:        "get_weather",
				Description: "Returns the current temperature in Celsius for a city.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
					"required": []string{"city"},
				},
			},
		},
		{
			Type: "function",
			Function: chat.FunctionDef{
				Name:        "add_numbers",
				Description: "Adds two numbers and returns their sum.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"a": map[string]any{"type": "number"},
						"b": map[string]any{"type": "number"},
					},
					"required": []string{"a", "b"},
				},
			},
		},
	}

	handlers := map[string]chat.Handler{
		"get_weather": func(ctx context.Context, args string) (string, error) {
			var p struct{ City string }
			json.Unmarshal([]byte(args), &p)
			// Stub: return a fixed value for any city.
			return fmt.Sprintf(`{"city":%q,"temperature_c":22}`, p.City), nil
		},
		"add_numbers": func(ctx context.Context, args string) (string, error) {
			var p struct{ A, B float64 }
			json.Unmarshal([]byte(args), &p)
			return fmt.Sprintf(`{"sum":%g}`, p.A+p.B), nil
		},
	}

	comp, err := client.Chat.RunAgent(
		context.Background(),
		&chat.CreateRequest{
			Model: "grok-4-1-fast-reasoning",
			Messages: []chat.Message{
				{Role: "user", Content: "What is the weather in Paris, and what is 17 + 25?"},
			},
			Tools:      tools,
			ToolChoice: "auto",
		},
		handlers,
		chat.WithMaxTurns(5),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(comp.Choices[0].Message.Content)
	fmt.Printf("\ntokens: %d prompt / %d completion\n",
		comp.Usage.PromptTokens, comp.Usage.CompletionTokens)
}
