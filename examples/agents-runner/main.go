// agents-runner demonstrates agents.Runner, the structured agent harness
// that captures a Trajectory of every turn.
//
// Use Runner instead of chat.RunAgent when you want:
//   - Per-turn observability (ToolCallTrace, durations, errors)
//   - Multiple agents that hand off to each other
//   - Input/output guardrails
//   - A persistent Session across runs
//   - Middleware to wrap every Run with cross-cutting behavior
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

	weather := agents.Tool{
		Name:        "get_weather",
		Description: "Returns the current temperature in Celsius for a city.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city": map[string]any{"type": "string"},
			},
			"required": []string{"city"},
		},
		Handler: func(_ context.Context, args string) (string, error) {
			var p struct{ City string }
			json.Unmarshal([]byte(args), &p)
			return fmt.Sprintf(`{"city":%q,"temp_c":22}`, p.City), nil
		},
	}

	agent := &agents.Agent{
		Name:         "WeatherBot",
		Instructions: "You are a concise assistant. Always call get_weather before answering.",
		Model:        "grok-4-1-fast-reasoning",
		Tools:        []agents.Tool{weather},
	}

	runner := &agents.Runner{Client: client.Chat, MaxTurns: 5}

	res, err := runner.Run(context.Background(), agent, agents.RunOptions{
		Input: "What's the weather in Tokyo right now?",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("Final answer:", res.Output)
	fmt.Println()
	fmt.Printf("Trajectory: run_id=%s turns=%d duration=%dms total_tokens=%d\n",
		res.Trajectory.RunID,
		len(res.Trajectory.Turns),
		res.Trajectory.EndedAt.Sub(res.Trajectory.StartedAt).Milliseconds(),
		res.Usage.TotalTokens,
	)
	for _, t := range res.Trajectory.Turns {
		fmt.Printf("  turn %d (agent=%s, %dms)\n", t.Index, t.AgentName, t.DurationMS)
		for _, tc := range t.ToolCalls {
			fmt.Printf("    tool: %s(%s) -> %s [%dms]\n", tc.Name, tc.ArgsJSON, tc.Result, tc.DurationMS)
		}
	}
}
