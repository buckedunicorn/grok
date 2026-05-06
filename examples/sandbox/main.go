// sandbox demonstrates wiring agents/sandbox.DockerExecutor into
// agents/harness so the agent's shell tool runs inside an ephemeral
// container instead of on the host.
//
// Default LocalExecutor (the harness fallback) gives the agent the same
// authority as the parent process, fine for local-developer use, dangerous
// once you start handing arbitrary tasks to a model. DockerExecutor swaps
// that for a `docker run --rm` per command with hardened defaults:
// network=none, cap-drop=ALL, no-new-privileges, read-only root with
// /tmp tmpfs, pids-limit=256, user=65534:65534. Override with NetworkOn,
// AllowSetUID, WritableRoot, Caps, PIDsLimit, User as needed.
//
// Requirements:
//   - docker on $PATH
//   - the configured Image cached locally (or set PullPolicy: "always" if you're OK with a network round-trip on first call)
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/agents/harness"
	"github.com/buckedunicorn/grok/agents/sandbox"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	exec := &sandbox.DockerExecutor{
		Image:    "alpine:3.20",
		MemoryMB: 256,
		CPUs:     1.0,
		// network=none, cap-drop=ALL, no-new-privileges, read-only
		// root, pids-limit=256, user=65534:65534 (nobody) all default
		// on; nothing else to set for an untrusted-code workload.
		// Mounts: map[string]string{"/path/on/host": "/data"}, // read-only
	}

	if err := exec.Available(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "install Docker (https://docs.docker.com/get-docker/) and try again.")
		os.Exit(1)
	}

	h := harness.NewDefault(client.Chat, "grok-4-1-fast-reasoning",
		harness.WithExecutor(exec),
		harness.WithMaxTurns(10),
	)

	task := `Use the execute tool to run the following commands inside the sandbox:
1. uname -a (to confirm we're in alpine)
2. whoami (to confirm we're not root)
3. echo "hello from inside the container"

Then summarize what you observed.`

	res, err := h.Run(context.Background(), task)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("=== Final answer ===")
	fmt.Println(res.Output)
	fmt.Println()
	fmt.Println("=== Tool calls ===")
	for _, t := range res.Trajectory.Turns {
		for _, tc := range t.ToolCalls {
			status := "ok"
			if tc.Err != nil {
				status = "err"
			}
			fmt.Printf("  turn %d  %-12s %s  [%dms]\n", t.Index, tc.Name, status, tc.DurationMS)
		}
	}
}
