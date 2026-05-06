// agents-handoff demonstrates Handoff, one agent transferring control to
// another mid-run. The Triage agent receives the user's question; if it's
// math, Triage hands off to MathBot, otherwise it answers directly.
//
// The Trajectory records the HandoffTrace so downstream tools can attribute
// each turn to the correct agent.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/agents"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	mul := agents.Tool{
		Name:        "multiply",
		Description: "Multiplies two numbers.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"a": map[string]any{"type": "number"},
				"b": map[string]any{"type": "number"},
			},
			"required": []string{"a", "b"},
		},
		Handler: func(_ context.Context, args string) (string, error) {
			var p struct{ A, B float64 }
			json.Unmarshal([]byte(args), &p)
			return fmt.Sprintf(`%g`, p.A*p.B), nil
		},
	}

	mathBot := &agents.Agent{
		Name:         "MathBot",
		Instructions: "You answer numeric questions using the multiply tool when needed.",
		Model:        "grok-4-1-fast-reasoning",
		Tools:        []agents.Tool{mul},
	}

	triage := &agents.Agent{
		Name: "Triage",
		Instructions: `You are a triage assistant. If the user's question requires
arithmetic, call hand_to_math to delegate. Otherwise answer directly.`,
		Model: "grok-4-1-fast-reasoning",
		Handoffs: []agents.Handoff{{
			Name:        "hand_to_math",
			Description: "Hand control to MathBot for arithmetic questions.",
			Agent:       mathBot,
		}},
	}

	runner := &agents.Runner{Client: client.Chat, MaxTurns: 6}
	res, err := runner.Run(context.Background(), triage, agents.RunOptions{
		Input: "What is 23 multiplied by 17?",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("Final answer:", res.Output)
	fmt.Println("Last agent: ", res.LastAgent.Name)
	fmt.Println()
	fmt.Println("Trajectory:")
	for _, t := range res.Trajectory.Turns {
		fmt.Printf("  turn %d (agent=%s)\n", t.Index, t.AgentName)
		if t.Handoff != nil {
			fmt.Printf("    HANDOFF: %s -> %s (reason=%s)\n", t.Handoff.From, t.Handoff.To, t.Handoff.Reason)
		}
		for _, tc := range t.ToolCalls {
			fmt.Printf("    tool: %s(%s) -> %s\n", tc.Name, tc.ArgsJSON, tc.Result)
		}
	}
}
