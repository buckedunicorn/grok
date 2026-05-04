// decode demonstrates chat.Decode[T] for structured JSON output.
// The model is asked to respond in JSON; Decode unmarshals it into a typed struct
// without a separate json.Unmarshal call.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
)

// Recipe is the expected output structure.
type Recipe struct {
	Name        string   `json:"name"`
	Ingredients []string `json:"ingredients"`
	Steps       []string `json:"steps"`
	TimeMinutes int      `json:"time_minutes"`
}

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	schema := map[string]any{
		"name":   "recipe",
		"strict": true,
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":         map[string]any{"type": "string"},
				"ingredients":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"steps":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"time_minutes": map[string]any{"type": "integer"},
			},
			"required": []string{"name", "ingredients", "steps", "time_minutes"},
		},
	}
	schemaRaw, _ := json.Marshal(schema)
	rawMsg := json.RawMessage(schemaRaw)

	comp, err := client.Chat.Create(context.Background(), &chat.CreateRequest{
		Model: "grok-4-1-fast-reasoning",
		Messages: []chat.Message{
			{Role: "system", Content: "Respond only with valid JSON matching the provided schema."},
			{Role: "user", Content: "Give me a simple recipe for scrambled eggs."},
		},
		ResponseFormat: &chat.ResponseFormat{
			Type:       "json_schema",
			JSONSchema: &rawMsg,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	recipe, err := chat.Decode[Recipe](comp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}

	fmt.Printf("Recipe: %s (%d min)\n\n", recipe.Name, recipe.TimeMinutes)
	fmt.Println("Ingredients:")
	for _, ing := range recipe.Ingredients {
		fmt.Printf("  - %s\n", ing)
	}
	fmt.Println("\nSteps:")
	for i, step := range recipe.Steps {
		fmt.Printf("  %d. %s\n", i+1, step)
	}
}
