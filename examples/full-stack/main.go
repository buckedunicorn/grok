// full-stack composes every advanced agent feature into one runnable demo:
//
//   - harness    , opinionated default agent with planning + FS + shell + subagent tools
//   - sandbox    , DockerExecutor runs every shell command in an ephemeral container
//   - tracing    , OpenTelemetry stdout exporter; spans print as JSON during the run
//   - durable    , FileStore checkpoints after every tool-call turn; resumable
//
// Pass -resume <runID> to continue an interrupted run from its last
// checkpoint instead of starting fresh.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/agents/durable"
	"github.com/buckedunicorn/grok/agents/harness"
	"github.com/buckedunicorn/grok/agents/sandbox"
	"github.com/buckedunicorn/grok/agents/tracing"
)

func main() {
	resumeID := flag.String("resume", "", "resume the given runID instead of starting a new run")
	checkpointDir := flag.String("dir", filepath.Join(os.TempDir(), "grok-full-stack-checkpoints"), "checkpoint storage directory")
	workspaceDir := flag.String("workspace", "", "agent workspace dir (default: a fresh temp dir per run; pass the same dir on resume)")
	flag.Parse()

	ctx := context.Background()

	// --- Tracing: stdout exporter (swap for OTLP in production) ---
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		fatal(err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer func() { _ = tp.Shutdown(ctx) }()
	otel.SetTracerProvider(tp)
	tracer := tp.Tracer("github.com/buckedunicorn/grok/examples/full-stack")

	// --- Durable: FileStore for checkpoints ---
	store, err := durable.NewFileStore(*checkpointDir)
	if err != nil {
		fatal(err)
	}

	// --- Workspace: same dir on resume so the FS state matches the saved Trajectory ---
	workspace := *workspaceDir
	if workspace == "" {
		workspace, err = os.MkdirTemp("", "grok-full-stack-workspace-")
		if err != nil {
			fatal(err)
		}
	}
	fmt.Println("Workspace:     ", workspace)
	fmt.Println("Checkpoint dir:", *checkpointDir)

	// --- Sandbox executor ---
	// network=none, cap-drop=ALL, no-new-privileges, read-only root,
	// pids-limit=256 are all default on. We override User to the host
	// uid:gid so files written into /workspace are owned by the
	// running user rather than 65534:65534.
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
		fatal(fmt.Errorf("docker: %w", err))
	}

	// --- LocalFS rooted in the workspace ---
	fs, err := harness.NewLocalFS(workspace)
	if err != nil {
		fatal(err)
	}

	// --- Harness with all four feature wires attached ---
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))
	h := harness.NewDefault(client.Chat, "grok-4-1-fast-reasoning",
		harness.WithFS(fs),
		harness.WithExecutor(exec),
		harness.WithMaxTurns(20),
		harness.WithHooks(tracing.Hooks(tracer)),
		harness.WithMiddleware(tracing.Middleware(tracer)),
		harness.WithCheckpointer(store),
		harness.WithSubagent(false), // keep the trace tree shallow for readability
	)

	var (
		res     *agents.RunResult
		runErr  error
		runMode string
	)
	if *resumeID != "" {
		runMode = "resumed"
		fmt.Println("\nResuming run", *resumeID)
		state, err := store.Load(ctx, *resumeID)
		if err != nil {
			fatal(err)
		}
		res, runErr = h.Resume(ctx, state)
	} else {
		runMode = "fresh"
		fmt.Println("\nStarting fresh run")
		res, runErr = h.Run(ctx, defaultTask)
	}

	if runErr != nil {
		fmt.Fprintln(os.Stderr, "\nrun error:", runErr)
		ids, _ := store.List(ctx)
		if len(ids) > 0 {
			fmt.Fprintln(os.Stderr, "checkpoints saved:", ids)
			fmt.Fprintf(os.Stderr, "resume with:\n  go run ./examples/full-stack -resume %s -dir %s -workspace %s\n",
				ids[len(ids)-1], *checkpointDir, workspace)
		}
		os.Exit(1)
	}

	fmt.Printf("\n=== %s run complete ===\n", runMode)
	fmt.Println(res.Output)
	fmt.Printf("\nrun_id=%s  turns=%d  total_tokens=%d  duration=%dms\n",
		res.Trajectory.RunID,
		len(res.Trajectory.Turns),
		res.Usage.TotalTokens,
		res.Trajectory.EndedAt.Sub(res.Trajectory.StartedAt).Milliseconds(),
	)

	// On a successful run, the saved checkpoint is no longer needed.
	if err := store.Delete(ctx, res.Trajectory.RunID); err != nil {
		fmt.Fprintln(os.Stderr, "checkpoint cleanup:", err)
	}
}

const defaultTask = `Write a Python script that:
1. Computes the sum of squares from 1 to 100.
2. Computes the square of the sum from 1 to 100.
3. Prints the absolute difference between the two (Project Euler #6).

Run the script and reply with just the difference.`

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
