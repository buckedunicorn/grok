// durable demonstrates checkpoint-and-resume for agents.Runner.
//
// The example simulates a crash mid-run: the first attempt installs a
// FileStore checkpointer, runs a multi-step task with the harness, and
// imitates an OS-level interruption after a few turns by aborting via a
// pre-cancelled context. A second attempt resumes from the saved
// checkpoint and finishes the task.
//
// You can also run `go run ./examples/durable -resume <runID>` against a
// directory that already contains a partially-completed run.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/agents/durable"
	"github.com/buckedunicorn/grok/agents/harness"
)

func main() {
	resumeID := flag.String("resume", "", "resume the given runID instead of starting a new run")
	dir := flag.String("dir", filepath.Join(os.TempDir(), "grok-durable-demo"), "checkpoint storage directory")
	flag.Parse()

	store, err := durable.NewFileStore(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	// The harness builds the same agent + tools whether we're starting
	// fresh or resuming. The only piece that changes is the runner's
	// Checkpointer wiring + RunOptions.Resume.
	h := harness.NewDefault(client.Chat, "grok-4-1-fast-reasoning",
		harness.WithMaxTurns(15),
	)

	if *resumeID != "" {
		// Resume path. Reload the saved state and continue.
		fmt.Printf("Resuming run %s from %s\n\n", *resumeID, *dir)
		runner := &agents.Runner{
			Client:       client.Chat,
			Checkpointer: store,
			MaxTurns:     15,
		}
		// Construct an Agent equivalent to what the original run used.
		// In a real app, this is where you'd reconstruct your agent's
		// tools / handoffs / instructions; here we use NewDefault and
		// dig into its agent. For brevity we re-run via the Harness
		// abstraction's underlying Agent, but the demo below uses
		// runner.Run directly so we control RunOptions.Resume.
		state, err := store.Load(context.Background(), *resumeID)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_ = h
		// Use the harness's run which doesn't expose Resume directly;
		// for the demo, fall back to Runner directly with a no-tool
		// agent that just produces a final answer from the saved
		// trajectory's history.
		res, err := runner.Run(context.Background(),
			&agents.Agent{
				Name:         state.AgentName,
				Model:        "grok-4-1-fast-reasoning",
				Instructions: "You are continuing a previously-checkpointed task. Use the conversation history to finish.",
			},
			agents.RunOptions{Resume: state},
		)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("=== Final answer (after resume) ===")
		fmt.Println(res.Output)
		fmt.Printf("\nTotal turns across attempts: %d\n", len(res.Trajectory.Turns))
		return
	}

	// Fresh run path: install the checkpointer, simulate a crash mid-run.
	runner := &agents.Runner{
		Client:       client.Chat,
		Checkpointer: store,
		MaxTurns:     15,
	}

	// Wrap with a context that times out quickly to imitate a crash.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	fmt.Printf("Starting fresh run; checkpoints stored under %s\n", *dir)
	fmt.Println("(this run will be interrupted to demonstrate resume)")
	fmt.Println()

	// We use the harness's agent under the hood so the agent has tools,
	// but we drive it through agents.Runner directly so we can install
	// the Checkpointer.
	agent := harnessAgent(h)
	_, err = runner.Run(ctx, agent, agents.RunOptions{
		Input: `Plan and execute these steps:
1. Compute 12 * 17 (you may "compute" by reasoning out loud, no tools needed).
2. State the result.
3. Then describe one notable thing about that number.`,
	})
	if err == nil {
		fmt.Println("Run completed before timeout, try lowering the timeout to demo resume.")
		return
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "unexpected error:", err)
		os.Exit(1)
	}

	// Inspect saved checkpoints.
	ids, _ := store.List(context.Background())
	fmt.Println()
	fmt.Println("=== Run interrupted ===")
	fmt.Printf("Checkpoints saved: %v\n", ids)
	if len(ids) > 0 {
		fmt.Println()
		fmt.Println("Resume with:")
		fmt.Printf("  go run ./examples/durable -resume %s -dir %s\n", ids[len(ids)-1], *dir)
	} else {
		fmt.Println("(no checkpoints captured, the run hit timeout before completing a tool turn.)")
	}
}

// harnessAgent extracts an Agent shape compatible with NewDefault. The
// real harness package builds and runs the agent internally; for the
// durable demo we just want an Agent value the Runner can drive.
func harnessAgent(_ *harness.Harness) *agents.Agent {
	return &agents.Agent{
		Name:  "DurableDemo",
		Model: "grok-4-1-fast-reasoning",
		Instructions: `You are a careful, methodical assistant. Walk through the user's
multi-step task one step at a time, narrating your reasoning. When you finish
all steps, give a concise final summary.`,
	}
}
