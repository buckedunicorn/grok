# agents

A structured agent runtime built on top of [`chat.Client`](../chat). Reach for `agents.Runner` instead of `chat.RunAgent` when you want observable runs, multi-agent handoffs, or input/output guardrails.

```go
import "github.com/buckedunicorn/grok/agents"
```

## When to use

| Use case | Reach for |
|---|---|
| One-shot tool calling, no observability | [`chat.Client.RunAgent`](../chat) |
| Observable runs with `Trajectory`, hooks, middleware | `agents.Runner` |
| Pre-wired planning, FS, shell, subagent tools | [`agents/harness`](./harness) |
| Resumable runs with checkpoint persistence | [`agents/durable`](./durable) |
| Score and aggregate `RunResult` values | [`agents/eval`](./eval) |
| Sandboxed shell execution | [`agents/sandbox`](./sandbox) |
| OpenTelemetry instrumentation | [`agents/tracing`](./tracing) |

## Surface

| Type | Role |
|---|---|
| `Agent` | A configured persona: instructions, tools, handoffs, guardrails |
| `Runner` | Drives the loop. `Run(ctx, input)` returns a `RunResult` |
| `Tool` | Function tool with name, description, parameters schema, handler |
| `Handoff` | Lets one agent hand control to another mid-run |
| `Session` | Conversation state. `InMemorySession` is the in-process default; `SessionFromConversation` adapts an existing `chat.Conversation` |
| `Guardrails` | Input and output filters (regex, custom) that can short-circuit a run |
| `RunHooks` | Lifecycle callbacks (turn started, tool called, handoff, ...) |
| `Middleware` | Wraps each `Run` call (e.g., for tracing, retries, rate limiting) |
| `RunResult` | Output, usage, and a structured `Trajectory` (turns, tool calls, handoffs, guardrail decisions) |
| `Trajectory` | Offline-analyzable record of a run; what `eval` and `tracing` consume |

## Example

```go
import (
    "github.com/buckedunicorn/grok"
    "github.com/buckedunicorn/grok/agents"
)

client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

agent := &agents.Agent{
    Name:         "researcher",
    Model:        "grok-4-1-fast-reasoning",
    Instructions: "You are a careful research assistant. Cite sources.",
    Tools: []agents.Tool{searchTool, summariseTool},
}

runner := agents.NewRunner(client.Chat)
res, err := runner.Run(ctx, agent, "Summarise the latest paper on diffusion models.")
if err != nil { return err }

fmt.Println(res.Output)
for _, turn := range res.Trajectory.Turns {
    fmt.Printf("turn %d: %d tool calls\n", turn.Index, len(turn.ToolCalls))
}
```

## Threat model

When you give an agent tools that drive code execution, filesystem access, or network access, the model becomes part of the trust boundary. The runtime cannot tell the difference between a benign tool call and one driven by prompt injection from a third-party message inside the conversation.

What `agents.Runner` provides:

- **Bounded loops:** `MaxTurns` caps the number of model rounds (default 10). `ErrMaxTurnsExceeded` is returned with a partial `RunResult`.
- **Guardrails:** input and output guardrails run per turn and can short-circuit a run via `ErrGuardrailTripped`. The trace records the verdict.
- **Trajectory redaction:** `RunHooks.RedactToolArgs` and `RunHooks.RedactToolResult` are called before tool arguments and results are recorded. Use them to strip secrets before the strings land in checkpoints, OpenTelemetry spans, or eval reports.
- **No automatic sandboxing:** the runtime does not isolate tool execution by itself. That is the operator's job.

What you must add when exposing tools to a model:

- **Sandbox shell tools.** Use [`agents/sandbox.DockerExecutor`](./sandbox) (default-deny network, cap-drop=ALL, no-new-privileges, read-only root, pids-limit, nobody UID, capped output) instead of [`agents/harness.LocalExecutor`](./harness) for any agent that processes untrusted input.
- **Confine filesystem tools.** Use [`agents/harness.LocalFS`](./harness), which is rooted at a single directory via `os.Root` (kernel-enforced containment, symlink-following blocked).
- **Validate URL-fetching tools.** Apply an SSRF guard equivalent to [`discord.FetchAsDataURI`'s](../discord) (https-only, IP-range check, redirect re-validation, dial-time DNS guard) before fetching any model-supplied URL.
- **Treat user input as untrusted.** Add a layer of input validation, rate limiting, and content scoping in front of the runner. The runtime does not sanitise on your behalf.
- **Treat tool descriptions as input to the model.** A tool registered with attacker-controlled `Description` text can attempt to manipulate the model. Only register tools whose descriptions you control.
- **Set `Checkpointer` paths carefully.** [`agents/durable.FileStore`](./durable) writes per-run JSON to disk at mode 0o600 inside an `os.Root`-confined dir. Do not place that dir on shared writable paths.

For the SDK's full security posture see [`SECURITY.md`](../SECURITY.md).

## Trajectory

Every `Runner.Run` returns a `RunResult.Trajectory`. The trajectory captures every turn, tool call, handoff, and guardrail decision in a structured form. This makes runs:

- Reproducible (replay the trajectory deterministically without hitting the model)
- Scoreable (the `eval` package consumes trajectories)
- Resumable (the `durable` package serialises checkpoints between turns)
- Observable (the `tracing` package emits OpenTelemetry spans for each entry)

## Examples directory

- [`examples/agents-runner`](../examples/agents-runner)
- [`examples/agents-handoff`](../examples/agents-handoff)
- [`examples/agents-guardrails`](../examples/agents-guardrails)
- [`examples/harness`](../examples/harness)
- [`examples/coding-agent`](../examples/coding-agent)
- [`examples/durable`](../examples/durable)
- [`examples/eval`](../examples/eval)
- [`examples/sandbox`](../examples/sandbox)
- [`examples/sandboxed-eval`](../examples/sandboxed-eval)
- [`examples/tracing`](../examples/tracing)
- [`examples/full-stack`](../examples/full-stack)
