// coding-agent demonstrates a sandboxed coding agent: harness gives it
// filesystem and shell tools, sandbox.DockerExecutor runs every shell
// command inside an ephemeral python:3.12-alpine container with the
// workspace mounted read-write.
//
// The agent's filesystem and shell point at the SAME directory: the
// LocalFS lets it write source files (which the host user can inspect
// after the run), and the DockerExecutor mounts that directory into the
// container so `python3 solution.py` reads what the agent just wrote.
//
// Hardening defaults applied (the first five are now
// DockerExecutor's built-in defaults; we just add MemoryMB / CPUs /
// User on top):
//   - --network=none                 (no DNS / no outbound; default)
//   - --cap-drop=ALL                  (default)
//   - --security-opt=no-new-privileges (default)
//   - --read-only with /tmp tmpfs    (default)
//   - --pids-limit=256                (default)
//   - MemoryMB:  256                  (bounded memory blast radius)
//   - CPUs:      1.0                  (bounded CPU)
//   - User:      host UID             (files written in-container are
//     owned by the same user that owns the workspace, so cleanup
//     doesn't need root)
//
// Requirements: docker on $PATH, python:3.12-alpine cached locally
// (`docker pull python:3.12-alpine`).
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/agents/harness"
	"github.com/buckedunicorn/grok/agents/sandbox"
)

func main() {
	// 1) workspace on the host, files persist after the run for inspection
	workspace, err := os.MkdirTemp("", "grok-coding-agent-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Workspace:", workspace)
	fmt.Println("(after the run, try: ls", workspace, "&& cat", workspace+"/solution.py)")
	fmt.Println()

	// 2) FS rooted at the workspace
	fs, err := harness.NewLocalFS(workspace)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// 3) Docker executor with the workspace mounted RW. User matches the
	//    host so files written by `python3` inherit the host user's perms.
	//    Network and other hardening flags default on; nothing to set.
	hostUser := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	exec := &sandbox.DockerExecutor{
		Image:    "python:3.12-alpine",
		Workdir:  "/workspace",
		MountsRW: map[string]string{workspace: "/workspace"},
		MemoryMB: 256,
		CPUs:     1.0,
		User:     hostUser,
	}
	if err := exec.Available(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "install Docker (https://docs.docker.com/get-docker/) and pull python:3.12-alpine.")
		os.Exit(1)
	}

	// 4) Harness wires it all together. MaxTurns(30) is generous; reasoning
	//    models often spend several turns on planning + tool retries before
	//    landing the final answer.
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))
	h := harness.NewDefault(client.Chat, "grok-4-1-fast-reasoning",
		harness.WithFS(fs),
		harness.WithExecutor(exec),
		harness.WithMaxTurns(30),
		harness.WithSubagent(false),
		harness.WithInstructions(`You are a careful coding assistant working in a Python sandbox.

Available tools:
- write_file / read_file / edit_file / ls / glob / grep, workspace files (host-visible)
- execute, runs commands inside the python:3.12-alpine sandbox; the workspace is mounted at /workspace, which is also the working directory

Workflow for any coding task:
1. (Optional) write_todos once at the start to outline 2-4 steps. Don't update it on every turn.
2. write_file to create the source.
3. execute to run it. ALWAYS pass {"cmd":"python3","args":["solution.py"]}, cmd is the executable name only, args is the arg list.
4. If execute returned a non-zero exit_code, inspect the result's stderr field and edit_file to fix; re-run.
5. As soon as the program prints what the user asked for, REPLY with the answer. Do NOT call any more tools after you have it.`),
	)

	task := `Write a Python script called solution.py that prints the first 20
prime numbers, one per line. Then run it with python3 and tell me what was
printed (the first three and the last three lines are enough). The whole
thing should take fewer than 50 lines of code.`

	res, err := h.Run(context.Background(), task)
	if err != nil {
		fmt.Fprintln(os.Stderr, "\nrun error:", err)
		// res is non-nil even on error so callers can inspect the
		// Trajectory and figure out what happened.
		if res != nil {
			printTrajectory(res)
		}
		os.Exit(1)
	}

	fmt.Println("=== Final answer ===")
	fmt.Println(res.Output)
	fmt.Println()
	printTrajectory(res)

	entries, _ := fs.List("")
	fmt.Println("\nWorkspace contents:")
	for _, e := range entries {
		fmt.Println(" ", workspace+"/"+e)
	}
}

// printTrajectory writes a compact per-turn summary so callers can audit
// which tools fired (and which failed) when debugging a run.
func printTrajectory(res *agents.RunResult) {
	fmt.Printf("Turns: %d   Total tokens: %d   Duration: %dms\n",
		len(res.Trajectory.Turns),
		res.Usage.TotalTokens,
		res.Trajectory.EndedAt.Sub(res.Trajectory.StartedAt).Milliseconds())

	fmt.Println("\n=== Tool calls ===")
	for _, t := range res.Trajectory.Turns {
		if t.Err != nil {
			fmt.Printf("  turn %d  TURN-ERR     %v\n", t.Index, t.Err)
		}
		for _, tc := range t.ToolCalls {
			status := "ok"
			if tc.Err != nil {
				status = "err"
			}
			fmt.Printf("  turn %d  %-12s %s  [%dms]  %s\n",
				t.Index, tc.Name, status, tc.DurationMS, summarize(tc.Result))
		}
	}
}

func summarize(s string) string {
	const max = 80
	s = strings.ReplaceAll(s, "\n", `\n`)
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
