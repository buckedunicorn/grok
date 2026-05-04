// council demonstrates council.Roundtable and council.Synthesize.
// Three agents with different personas answer the same question in parallel;
// a moderator then synthesizes their answers into a single response.
package main

import (
	"context"
	"fmt"
	"os"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/council"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	participants := []council.Participant{
		{
			Name:   "Optimist",
			Client: client.Chat,
			Model:  "grok-4-1-fast-reasoning",
			System: "You are an eternal optimist. Focus only on the upsides.",
		},
		{
			Name:   "Pessimist",
			Client: client.Chat,
			Model:  "grok-4-1-fast-reasoning",
			System: "You are a cautious realist who highlights risks and downsides.",
		},
		{
			Name:   "Engineer",
			Client: client.Chat,
			Model:  "grok-4-1-fast-reasoning",
			System: "You are a pragmatic engineer. Focus on technical feasibility and implementation.",
		},
	}

	question := "Should our team migrate our backend from Python to Go?"
	ctx := context.Background()

	// --- Roundtable: all three answer in parallel ---
	fmt.Printf("Question: %s\n\n", question)
	responses, err := council.Roundtable(ctx, question, participants)
	if err != nil {
		fmt.Fprintln(os.Stderr, "roundtable:", err)
		os.Exit(1)
	}

	for _, r := range responses {
		if r.Err != nil {
			fmt.Printf("[%s] ERROR: %v\n\n", r.Name, r.Err)
			continue
		}
		fmt.Printf("[%s]\n%s\n\n", r.Name, r.Output)
	}

	// --- Synthesize: a moderator distills the debate ---
	moderator := council.Participant{
		Name:   "Moderator",
		Client: client.Chat,
		Model:  "grok-4-1-fast-reasoning",
		System: "You are a balanced technical lead. Synthesize multiple perspectives into a concise recommendation.",
	}

	fmt.Println("--- Synthesis ---")
	synthesis, err := council.Synthesize(ctx, question, participants, moderator)
	if err != nil {
		fmt.Fprintln(os.Stderr, "synthesize:", err)
		os.Exit(1)
	}
	fmt.Println(synthesis)
}
