# agents-guardrails

Input and output guardrails on an Agent.

## Run

```sh
XAI_API_KEY=... go run ./examples/agents-guardrails
```

## What it shows

- `agents.InputGuardrail` runs before the first turn; returning an error aborts the run with `agents.ErrGuardrailTripped`
- `agents.OutputGuardrail` runs on the final assistant text before the run returns; same abort semantics
- Use `errors.Is(err, agents.ErrGuardrailTripped)` to distinguish guardrail trips from other errors
- Each guardrail check is recorded as a `GuardrailTrace` on the corresponding turn so downstream tooling can audit which checks fired
