// sugar demonstrates the optional langchain-style helpers, prompt
// templates, runnable composition, and output parsers, wired into a
// single typed pipeline.
//
// Pipeline shape:
//
//	map[string]any --(template)--> []chat.Message --(call)--> *chat.Completion --(parse)--> Recipe
//
// Each stage is a runnable.Runnable so the whole thing composes with
// runnable.Pipe3 and is invoked with one Run call.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/prompt"
	"github.com/buckedunicorn/grok/runnable"
)

// Recipe is the expected shape of the structured output.
type Recipe struct {
	Name        string   `json:"name"`
	Ingredients []string `json:"ingredients"`
	Steps       []string `json:"steps"`
	TimeMinutes int      `json:"time_minutes"`
}

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	// 1) prompt template, System + User pair with {dish} substitution.
	// NewChatTemplate parses both templates up front, so any syntax
	// error surfaces here rather than on the first Render.
	tmpl, err := prompt.NewChatTemplate(
		"Respond only with valid JSON matching the recipe schema.",
		"Give me a simple recipe for {dish}.",
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// 2) JSON schema for structured output
	schema, _ := json.Marshal(map[string]any{
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
	})
	schemaRaw := json.RawMessage(schema)

	// --- Stage A: render template into chat messages
	render := runnable.Func[map[string]any, []chat.Message](func(_ context.Context, vars map[string]any) ([]chat.Message, error) {
		return tmpl.Render(vars)
	})

	// --- Stage B: send to the model
	call := runnable.Func[[]chat.Message, *chat.Completion](func(ctx context.Context, msgs []chat.Message) (*chat.Completion, error) {
		return client.Chat.Create(ctx, &chat.CreateRequest{
			Model:    "grok-4-1-fast-reasoning",
			Messages: msgs,
			ResponseFormat: &chat.ResponseFormat{
				Type:       "json_schema",
				JSONSchema: &schemaRaw,
			},
		})
	})

	// --- Stage C: decode the JSON
	parse := runnable.Func[*chat.Completion, Recipe](func(_ context.Context, c *chat.Completion) (Recipe, error) {
		return chat.Decode[Recipe](c)
	})

	// --- Compose and run
	pipeline := runnable.Pipe3(render, call, parse)

	recipe, err := pipeline.Run(context.Background(), map[string]any{
		"dish": "scrambled eggs",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("Recipe: %s (%d min)\n\n", recipe.Name, recipe.TimeMinutes)
	fmt.Println("Ingredients:")
	for _, ing := range recipe.Ingredients {
		fmt.Printf("  - %s\n", ing)
	}
	fmt.Println("\nSteps:")
	for i, s := range recipe.Steps {
		fmt.Printf("  %d. %s\n", i+1, s)
	}
}
