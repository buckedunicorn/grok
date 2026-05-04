// eval demonstrates agents/eval, score a Suite of tasks against an
// agent and print an aggregate pass@1 / latency / token report.
//
// The example runs three small tasks against a basic agents.Runner agent
// (no tools), each with a different scorer:
//
//  1. Substring match  (eval.Contains)
//  2. Regex match      (eval.Regex)
//  3. LLM-as-judge     (eval.LLMJudge, uses Grok itself as the judge)
//
// Suite runs them in parallel with Concurrency: 3, then Report.Print
// emits a tabwriter summary. Tweak the tasks or scorers to bench your
// own agents.
package main

import (
	"context"
	"fmt"
	"os"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/agents/eval"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	// The agent under test, plain reasoning model, no tools.
	agent := &agents.Agent{
		Name:         "Helper",
		Instructions: "Answer concisely. One sentence unless the user asks for more.",
		Model:        "grok-4-1-fast-reasoning",
	}
	runner := &agents.Runner{Client: client.Chat, MaxTurns: 3}

	// Define the tasks.
	tasks := []eval.Task{
		{
			ID:     "capital-france",
			Input:  "What is the capital of France? One word.",
			Expect: eval.Contains{Substr: "Paris", CaseInsensitive: true},
			Tags:   []string{"factual"},
		},
		{
			ID:     "fibonacci-10",
			Input:  "What is the 10th Fibonacci number, starting from F(1)=1, F(2)=1? Reply with just the number.",
			Expect: &eval.Regex{Pattern: `\b55\b`},
			Tags:   []string{"math"},
		},
		{
			ID:    "haiku",
			Input: "Write a haiku about Go programming.",
			Expect: eval.LLMJudge{
				Client: client.Chat,
				Model:  "grok-4-1-fast-non-reasoning",
				Rubric: "Pass if the output is a haiku (5-7-5 syllable structure across three lines) about Go programming.",
			},
			Tags: []string{"creative", "judged"},
		},
	}

	// Wire the runner into the eval Suite via a RunFunc.
	runFn := func(ctx context.Context, task eval.Task) (*agents.RunResult, error) {
		return runner.Run(ctx, agent, agents.RunOptions{Input: task.Input})
	}

	suite := &eval.Suite{Tasks: tasks, Concurrency: 3}
	report, err := suite.Run(context.Background(), runFn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	report.Print(os.Stdout)
}
