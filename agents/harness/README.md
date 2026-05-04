# agents/harness

An opinionated default agent on top of [`agents.Runner`](..). Pre-wires planning, filesystem, shell, and subagent tools so a coding or research agent works out of the box.

```go
import "github.com/buckedunicorn/grok/agents/harness"
```

## When to use

Use `harness.NewDefault` when you want a working agent without hand-rolling the tool roster. The four pre-wired tool families are the ones almost every coding/research agent ends up needing:

| Tool family | Names | Backed by |
|---|---|---|
| Planning | `write_todos`, `list_todos` | In-memory todo list |
| Filesystem | `read_file`, `write_file`, `edit_file`, `ls`, `glob`, `grep` | Pluggable `FS` interface (default: `MemoryFS`; opt into `LocalFS` for rooted disk access) |
| Shell | `execute` | Pluggable `Executor` interface (default: `LocalExecutor`, no sandbox; pair with `agents/sandbox.DockerExecutor` for isolation) |
| Subagent | `task` | Spawns a fresh `agents.Runner` with an isolated `Session` for delegated subtasks. Cannot recurse (no grandchildren) |

## Surface

| API | Purpose |
|---|---|
| `NewDefault(client, model, opts...)` | Construct a `Harness` with the four tool families wired in |
| `Harness.Run(ctx, input)` | Run the agent. Same return shape as `agents.Runner.Run` |
| `Harness.Resume(ctx, state)` | Resume from a checkpoint loaded via [`agents/durable`](../durable) |
| `WithFS`, `WithExecutor` | Swap the FS or shell implementation |
| `WithMaxTurns`, `WithSubagent` | Run-loop controls |
| `WithHooks`, `WithMiddleware`, `WithCheckpointer` | Plumbed through to the underlying Runner |

## Example

```go
import (
    grok "github.com/buckedunicorn/grok"
    "github.com/buckedunicorn/grok/agents/harness"
    "github.com/buckedunicorn/grok/agents/sandbox"
)

client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

fs, _ := harness.NewLocalFS("/tmp/agent-workspace")
exec := &sandbox.DockerExecutor{Image: "python:3.12-alpine", NetworkOff: true}

h := harness.NewDefault(client.Chat, "grok-4-1-fast-reasoning",
    harness.WithFS(fs),
    harness.WithExecutor(exec),
    harness.WithMaxTurns(20),
)

res, err := h.Run(ctx, "Write fibonacci.py and run it for n=10.")
if err != nil { return err }
fmt.Println(res.Output)
```

## Threat model

A `Harness` exposes filesystem, shell, and subagent tools to the model. The model decides when to call them, so for any agent that processes untrusted input you must:

1. **Pair the shell tool with a sandbox.** The default `LocalExecutor` runs commands directly on the host with the same authority as the parent process. For untrusted workloads, plug in [`agents/sandbox.DockerExecutor`](../sandbox) instead. Its defaults (network=none, cap-drop=ALL, no-new-privileges, read-only root with /tmp tmpfs, pids-limit=256, user=65534:65534, capped stdout/stderr) are calibrated for adversarial code.
2. **Confine the filesystem.** `harness.LocalFS` is rooted at a single directory via `os.Root`. Symlink traversal out of `Root` is rejected by the kernel. `LocalFS.Glob` cannot enumerate outside `Root`. `WriteFile` is capped at 32 MiB by default (`WithLocalFSMaxFileBytes`); files default to 0o600 mode. The default `MemoryFS` keeps state in process memory and is the safest option when you do not actually need disk persistence.
3. **Bound the loop.** `WithMaxTurns(n)` caps tool-call rounds. `ErrMaxTurnsExceeded` is returned with a partial trajectory.
4. **Subagents inherit your tool roster.** A `task` subagent gets a fresh `Session` but the same FS and Executor. Recursion is forbidden (`enableSubagent: false` in children) so a subagent cannot escape via grandchildren, but a malicious subagent can still write to the parent's FS or run shell commands. If you need stricter isolation, install a per-call FS at the parent level.
5. **Tool arguments and results are recorded into the trajectory.** Use `RunHooks.RedactToolArgs` and `RunHooks.RedactToolResult` (set via `harness.WithHooks`) to scrub secrets before they land in checkpoints or OpenTelemetry spans.

For the SDK's full security posture see [`SECURITY.md`](../../SECURITY.md).

## Composing with other agents/* sub-packages

`harness` is designed to compose: every option that `agents.Runner` accepts (hooks, middleware, checkpointer) is exposed via the harness too. The [`examples/full-stack`](../../examples/full-stack) example wires harness + sandbox + durable + tracing together in one runnable program.

## Examples directory

- [`examples/harness`](../../examples/harness)
- [`examples/coding-agent`](../../examples/coding-agent)
- [`examples/sandboxed-eval`](../../examples/sandboxed-eval)
- [`examples/full-stack`](../../examples/full-stack)
