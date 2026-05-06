// sandboxed-eval composes agents/eval with a sandboxed harness: every task
// runs through harness.NewDefault wired to a sandbox.DockerExecutor, so all
// code execution happens inside python:3.12-alpine containers. The eval
// suite then aggregates pass@1 / mean tokens / latency across the runs.
//
// Each task gives the agent a tiny coding problem and asserts that the
// final answer contains the expected output. Sandbox executor isolation +
// eval Suite measurement = a runnable mini-benchmark.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/agents/eval"
	"github.com/buckedunicorn/grok/agents/harness"
	"github.com/buckedunicorn/grok/agents/sandbox"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	// One Docker executor reused across all tasks. The mount points get
	// rebuilt per-task below so each run has its own workspace.
	hostUser := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	if err := (&sandbox.DockerExecutor{Image: "python:3.12-alpine"}).Available(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "install Docker and pull python:3.12-alpine.")
		os.Exit(1)
	}

	tasks := []eval.Task{
		{
			ID:    "primes-20",
			Input: `Write solution.py that prints the first 20 prime numbers separated by spaces on one line. Run it with python3 and reply with just the printed line.`,
			Expect: eval.AllOf{
				eval.Contains{Substr: "2 3 5 7"},
				eval.Contains{Substr: "67 71"}, // last few primes
			},
			Tags: []string{"math", "easy"},
		},
		{
			ID:    "fizzbuzz-15",
			Input: `Write solution.py implementing FizzBuzz for n=1..15. Run it. Reply with the program's output verbatim.`,
			Expect: eval.AllOf{
				eval.Contains{Substr: "FizzBuzz"},
				eval.Contains{Substr: "Fizz\n4\nBuzz"},
			},
			Tags: []string{"classics"},
		},
		{
			ID:     "factorial-10",
			Input:  `Write solution.py that computes 10! recursively and prints the integer result. Run it and reply with the answer.`,
			Expect: eval.Contains{Substr: "3628800"},
			Tags:   []string{"math", "recursion"},
		},
		{
			ID:    "palindrome",
			Input: `Write solution.py that defines is_palindrome(s) and prints the results for "level", "Hello", and "racecar", one boolean per line. Run it and reply with the three lines.`,
			Expect: eval.AllOf{
				eval.Contains{Substr: "True"},
				eval.Contains{Substr: "False"},
			},
			Tags: []string{"strings"},
		},
	}

	// One harness factory per task so each run gets its own workspace and
	// in-process FS. We ALSO use sequential execution (Concurrency: 1)
	// because every parallel run would spawn its own container, fine to
	// raise on a beefy machine, but easier to read sequentially.
	runFn := func(ctx context.Context, task eval.Task) (*agents.RunResult, error) {
		workspace, err := os.MkdirTemp("", "grok-sandboxed-eval-"+task.ID+"-")
		if err != nil {
			return nil, err
		}
		fs, err := harness.NewLocalFS(workspace)
		if err != nil {
			return nil, err
		}
		// network=none, cap-drop=ALL, no-new-privileges, read-only
		// root, pids-limit=256 default on; User is overridden to the
		// host uid:gid so /workspace writes are owned by the caller.
		exec := &sandbox.DockerExecutor{
			Image:    "python:3.12-alpine",
			Workdir:  "/workspace",
			MountsRW: map[string]string{workspace: "/workspace"},
			MemoryMB: 256,
			CPUs:     1.0,
			User:     hostUser,
		}
		h := harness.NewDefault(client.Chat, "grok-4-1-fast-reasoning",
			harness.WithFS(fs),
			harness.WithExecutor(exec),
			harness.WithSubagent(false), // keep it simple per task
			harness.WithMaxTurns(15),
			harness.WithInstructions(`You are a careful Python coding assistant. The container has python3 and the workspace is mounted at /workspace (also the cwd). For each task: write_todos, write_file solution.py, execute python3 solution.py, then reply with just the program output unless asked otherwise.`),
		)
		return h.Run(ctx, task.Input)
	}

	suite := &eval.Suite{Tasks: tasks, Concurrency: 1}
	report, err := suite.Run(context.Background(), runFn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	report.Print(os.Stdout)
}
