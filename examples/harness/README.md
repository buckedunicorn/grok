# harness

The opinionated default agent, `harness.NewDefault` returns a `Harness` preconfigured with planning, an in-memory filesystem, and subagent tools.

## Run

```sh
XAI_API_KEY=... go run ./examples/harness
```

## What it shows

- `harness.NewDefault(client, model)` → batteries-included agent in one line
- Default toolset: `write_todos`, `list_todos`, `read_file`, `write_file`, `edit_file`, `ls`, `glob`, `grep`, `task`
- Inspecting the structured `Trajectory` to audit every tool call (name, duration, error)
- Shell access is **opt-in** via `harness.WithExecutor(&harness.LocalExecutor{...})`, kept off by default because the default `LocalExecutor` runs commands with no sandbox

## A note on the filesystem

`NewDefault` wires a `MemoryFS` by default, files live in process memory and disappear when the run ends. That's the right choice for the package, but it means you won't find the agent's `haiku.txt` on disk after the run finishes.

This demo overrides the default with `harness.NewLocalFS(tempdir)` so you can `cat` the file afterward. The path is printed at the top of the run. Drop the `WithFS(...)` line to revert to the in-memory default.

## Customizing

| Override | Effect |
|---|---|
| `harness.WithFS(fs)` | Swap the in-memory FS for `harness.NewLocalFS("/abs/path")` (rooted, refuses traversal) or any custom `harness.FS` implementation |
| `harness.WithExecutor(e)` | Enable the `execute` tool with your own sandboxing `Executor` |
| `harness.WithPlanning(false)` | Drop the todo tools |
| `harness.WithSubagent(false)` | Drop the `task` tool (subagents recurse to depth 1 only by default) |
| `harness.WithInstructions(s)` | Replace the default system prompt |
| `harness.WithMaxTurns(n)` | Cap the underlying Runner's loop |
| `harness.WithHooks(h)` | Lifecycle callbacks on the underlying Runner |

For the full primitives layer (Agent, Runner, Tool, Handoff, Session, Guardrails, RunHooks, Middleware, Trajectory) see the `agents/` package directly, `harness/` sits on top of it.
