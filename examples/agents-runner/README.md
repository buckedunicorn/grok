# agents-runner

Tool-calling agent using `agents.Runner`, the structured harness that captures a `Trajectory` of every turn.

## Run

```sh
XAI_API_KEY=... go run ./examples/agents-runner
```

## What it shows

- Defining an `agents.Agent` with `Instructions`, `Model`, and `Tools`
- Running it with `Runner.Run` and inspecting the structured `RunResult.Trajectory`, per-turn `ToolCallTrace`, durations, agent attribution
- When to pick `agents.Runner` over `chat.RunAgent`: any time you want observable runs, handoffs, guardrails, sessions, or middleware. For a one-shot procedural loop with no trajectory needs, `chat.RunAgent` stays the lighter option.
