# workflow

Composable LLM pipelines with `workflow.Chain`, `workflow.Transform`, and friends.

## Run

```sh
XAI_API_KEY=... go run ./examples/workflow
```

## What it shows

- `workflow.Step` as the composable unit: any `func(ctx, string) (string, error)`
- `workflow.Chain` pipes steps sequentially, each step's output is the next step's input
- `workflow.Transform` wraps a pure string function as a `Step` (no API call needed)
- Mixing API-calling steps (summarise, translate) with local transforms (strings.ToUpper) in one chain
