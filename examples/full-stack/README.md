# full-stack

Every advanced agent feature composed into a single runnable demo: harness + sandbox + tracing + durable, all wired through `harness.Harness`.

## Run

```sh
docker pull python:3.12-alpine
XAI_API_KEY=... go run ./examples/full-stack
```

Spans print as pretty JSON during the run. The final answer prints last.

## Resume after a crash

If the run is interrupted (Ctrl-C, OOM, network blip, your laptop closes), the FileStore retains a checkpoint after every tool-call turn. The on-error message prints a ready-to-paste resume command:

```sh
go run ./examples/full-stack \
  -resume <runID> \
  -dir /tmp/grok-full-stack-checkpoints \
  -workspace /tmp/grok-full-stack-workspace-XXX
```

Pass the same `-workspace` so the agent's filesystem state matches the saved trajectory; otherwise its references to `solution.py` won't resolve.

## What it shows

- **`harness.NewDefault` orchestrating four feature wires.** `WithFS` (LocalFS), `WithExecutor` (DockerExecutor), `WithHooks` + `WithMiddleware` (tracing), `WithCheckpointer` (durable.FileStore). The harness threads them all into the underlying `agents.Runner` so callers don't have to.
- **The Harness API mirrors `agents.Runner`.** `h.Run(ctx, task)` for fresh runs; `h.Resume(ctx, state)` for continuing from a checkpoint. Both produce a `*agents.RunResult` with the same Trajectory shape.
- **Self-cleaning checkpoints.** A successful run deletes its own checkpoint on the way out, only crashed/cancelled runs persist a checkpoint to resume from.

## What you'll see in the output

Three things are interleaved on stdout:

1. **Span JSON**, batched by the OTel stdout exporter. Each turn produces an `agent.turn` start/end pair plus `agent.tool_call` + `agent.tool_call.result` per tool invocation.
2. **The agent's final answer** (the difference computed by the script, should be `25164150` for n=100).
3. **Run metadata**, run_id, turn count, total tokens, duration.

For production use, swap `stdouttrace` for `otlptracegrpc` or your vendor's exporter, the rest of the wiring stays the same.

## Sandboxing notes

`python:3.12-alpine` with `NetworkOff: true`, `MemoryMB: 256`, `CPUs: 1.0`, and the host UID applied via `User`. The agent can't reach the network, can't fork-bomb, and any file it writes is owned by the host user (no sudo to clean up). See `examples/sandbox` and `examples/coding-agent` for variations.
