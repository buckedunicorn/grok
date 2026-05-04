# agents-handoff

Multi-agent flow with `agents.Handoff`, Triage delegates to MathBot when the question requires arithmetic.

## Run

```sh
XAI_API_KEY=... go run ./examples/agents-handoff
```

## What it shows

- Defining two agents (Triage + MathBot) with distinct instructions
- Wiring a handoff: `Handoff{Name: "hand_to_math", Agent: mathBot}` on Triage
- Runner exposes the handoff to the model as a zero-arg function tool; when the model invokes it, the Runner swaps the active agent
- The `RunResult.Trajectory` records each turn's `AgentName` and a `HandoffTrace` on the turn where the swap happened
