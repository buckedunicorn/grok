# durable

Checkpoint-and-resume for `agents.Runner` via `agents/durable`.

## Run

```sh
# First attempt, runs until the demo's short timeout, then dies
XAI_API_KEY=... go run ./examples/durable

# Output ends with a "Resume with: ..." hint. Copy the runID from List.
XAI_API_KEY=... go run ./examples/durable -resume <runID> -dir /tmp/grok-durable-demo
```

## What it shows

- `durable.NewFileStore(dir)`, atomic JSON-per-runID checkpointing on disk
- `agents.Runner.Checkpointer = store`, Runner saves a `CheckpointState` after each tool-call turn
- `agents.RunOptions.Resume = state`, Runner picks up at `state.NextTurn` with `state.Messages` rehydrated as the session and `state.Trajectory.Turns` preserved
- Trajectory continuity: the resumed run's `Trajectory.RunID` matches the original; turn count spans both attempts
- `store.List(ctx)` for management; `store.Delete(ctx, runID)` to clean up

## What gets persisted

| Field | Purpose |
|---|---|
| `RunID` | Stable identity across resumes |
| `NextTurn` | Loop index to execute next |
| `AgentName` | Active agent at checkpoint time (after handoffs), informational |
| `Messages` | Session history (system prompt, user input, all tool/assistant turns) |
| `Trajectory` | Every completed Turn with ToolCallTraces, HandoffTraces, GuardrailTraces |
| `Usage` | Cumulative tokens across the run |

`Agent.Tools[].Handler` is **not** persisted, those are Go functions. On resume the caller passes the Agent with its tools re-wired. The Runner trusts the caller; `state.AgentName` is informational only.

## Storage backends

| Backend | Use for |
|---|---|
| `durable.NewMemoryStore()` | Tests, single-process workloads, no durability across crashes |
| `durable.NewFileStore(dir)` | Disk-backed, atomic via temp+rename, single-process safe |
| Custom (Postgres, Redis, S3, etc.) | Implement the four-method `durable.Store` interface |

The Runner-side contract is just `agents.Checkpointer.SaveCheckpoint(ctx, state)`. Anyone wanting only the save side can implement that single method without pulling in `agents/durable`.
