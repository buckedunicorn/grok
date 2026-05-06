// agents-guardrails demonstrates input and output guardrails on an Agent.
//
// Input guardrails run before the first turn and can abort the run with
// ErrGuardrailTripped. Output guardrails run on the final assistant text
// and can also abort the run before it returns.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/agents"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	noProfanity := agents.InputGuardrail{
		Name: "no-profanity",
		Run: func(_ context.Context, input string) error {
			banned := []string{"badword1", "badword2"}
			for _, w := range banned {
				if strings.Contains(strings.ToLower(input), w) {
					return fmt.Errorf("input contains banned word %q", w)
				}
			}
			return nil
		},
	}

	maxLen := agents.OutputGuardrail{
		Name: "max-output-length",
		Run: func(_ context.Context, output string) error {
			if len(output) > 500 {
				return fmt.Errorf("output too long (%d chars > 500)", len(output))
			}
			return nil
		},
	}

	agent := &agents.Agent{
		Name:         "Helper",
		Instructions: "Answer concisely. Never exceed 3 sentences.",
		Model:        "grok-4-1-fast-non-reasoning",
		Guardrails: agents.Guardrails{
			Input:  []agents.InputGuardrail{noProfanity},
			Output: []agents.OutputGuardrail{maxLen},
		},
	}

	runner := &agents.Runner{Client: client.Chat}
	res, err := runner.Run(context.Background(), agent, agents.RunOptions{
		Input: "Briefly: what is Go?",
	})
	if err != nil {
		if errors.Is(err, agents.ErrGuardrailTripped) {
			fmt.Fprintln(os.Stderr, "guardrail tripped:", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("Answer:", res.Output)
	fmt.Println()
	fmt.Println("Guardrail traces:")
	for _, t := range res.Trajectory.Turns {
		for _, g := range t.Guardrails {
			fmt.Printf("  [%s] %s, passed=%v\n", g.Stage, g.Name, g.Passed)
		}
	}
}
