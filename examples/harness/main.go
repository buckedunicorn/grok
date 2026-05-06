// harness demonstrates agents/harness, the opinionated default agent
// preconfigured with planning, filesystem, and subagent tools.
//
// Default tools registered by NewDefault:
//   - write_todos / list_todos  (planning)
//   - read_file / write_file / edit_file / ls / glob / grep  (filesystem)
//   - task  (spawn an isolated subagent)
//
// IMPORTANT, choosing a filesystem:
//
// NewDefault wires a MemoryFS by default, fast, scoped per-run, leaves no
// trace on disk. Great for tests and ephemeral workflows but means you
// cannot `cat` the files the agent writes after the run finishes.
//
// This demo overrides that with a LocalFS rooted at an os.MkdirTemp directory
// so you can verify the agent's output on disk. The temp directory path is
// printed before the run; inspect it with `ls` / `cat` after the run finishes.
//
// Shell access is opt-in via WithExecutor. The default Executor (LocalExecutor)
// runs commands directly on the host with no sandbox; production workloads
// should plug in a Docker, Firecracker, or other sandboxing Executor.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/agents/harness"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	// Create a real on-disk workspace so the demo's output is visible after
	// the run. Switch to harness.NewMemoryFS() (or just drop WithFS to use
	// the default MemoryFS) for fully ephemeral runs.
	workspace, err := os.MkdirTemp("", "grok-harness-demo-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fs, err := harness.NewLocalFS(workspace)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Workspace:", workspace)
	fmt.Println("(after the run, try: ls -la", workspace, "&& cat", workspace+"/haiku.txt)")
	fmt.Println()

	h := harness.NewDefault(client.Chat, "grok-4-1-fast-reasoning",
		harness.WithFS(fs),
		harness.WithMaxTurns(15),
		// Uncomment to enable shell access (no sandbox, local-developer use only):
		// harness.WithExecutor(&harness.LocalExecutor{Workdir: workspace}),
	)

	task := `Plan and execute these steps:
1. Write a file called "haiku.txt" containing a haiku about Go programming.
2. Read it back to verify what was written.
3. Tell me the contents.`

	res, err := h.Run(context.Background(), task)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("=== Final answer ===")
	fmt.Println(res.Output)
	fmt.Println()
	fmt.Printf("Turns: %d   Total tokens: %d   Duration: %dms\n",
		len(res.Trajectory.Turns),
		res.Usage.TotalTokens,
		res.Trajectory.EndedAt.Sub(res.Trajectory.StartedAt).Milliseconds())

	fmt.Println("\n=== Tool calls ===")
	for _, t := range res.Trajectory.Turns {
		for _, tc := range t.ToolCalls {
			status := "ok"
			if tc.Err != nil {
				status = "err"
			}
			fmt.Printf("  turn %d  %-12s %s  [%dms]\n", t.Index, tc.Name, status, tc.DurationMS)
		}
	}

	fmt.Println("\nWorkspace contents:")
	entries, _ := fs.List("")
	if len(entries) == 0 {
		fmt.Println("  (empty, the agent did not write anything)")
	}
	for _, e := range entries {
		fmt.Println(" ", workspace+"/"+e)
	}
}
