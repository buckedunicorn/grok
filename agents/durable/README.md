# agents/durable

Checkpoint persistence for [`agents.Runner`](..). Lets a long-running agent loop survive process restarts.

```go
import "github.com/buckedunicorn/grok/agents/durable"
```

## How it works

`Store` is the central interface. It satisfies `agents.Checkpointer` (so the Runner can save state during a run) and adds `Load`, `List`, and `Delete` for management. Two implementations ship with the SDK:

- `MemoryStore`: in-process. Useful for tests.
- `FileStore`: atomic JSON-per-runID via temp + rename. Suitable for single-host production use.

The Runner saves `CheckpointState` after every tool-call turn. Resume by loading the last checkpoint and calling `Runner.Run` with `RunOptions.Resume` set, or use the `Resume` helper:

```go
state, err := store.Load(ctx, runID)
if err != nil { return err }
res, err := durable.Resume(ctx, runner, agent, state)
```

## Surface

| Type / function | Role |
|---|---|
| `Store` | Persistent backend interface |
| `MemoryStore` | In-process implementation |
| `NewFileStore(dir)` | File-backed implementation |
| `Resume(ctx, runner, agent, state)` | Helper that calls `Runner.Run` with the loaded state |

## Example

```go
store, _ := durable.NewFileStore("/var/lib/agent-checkpoints")

runner := agents.NewRunner(client.Chat, agents.WithCheckpointer(store))
res, err := runner.Run(ctx, agent, "Long-running task...")
if err != nil {
    // process crashes, restarts, etc.
    state, _ := store.Load(ctx, lastKnownRunID)
    res, err = durable.Resume(ctx, runner, agent, state)
}

// Successful completion: clean up.
_ = store.Delete(ctx, res.Trajectory.RunID)
```

## Examples directory

- [`examples/durable`](../../examples/durable)
- [`examples/full-stack`](../../examples/full-stack) (composed with sandbox + tracing)
