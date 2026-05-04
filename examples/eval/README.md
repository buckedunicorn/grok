# eval

Score a `Suite` of tasks against an agent and print pass@1 / latency / token aggregates.

## Run

```sh
XAI_API_KEY=... go run ./examples/eval
```

## What it shows

- Defining `eval.Task`s with three different scorer types: `eval.Contains`, `eval.Regex`, and `eval.LLMJudge`
- Wiring `agents.Runner` into the `Suite` via a `RunFunc` adapter
- `Suite.Run` executes tasks in parallel (`Concurrency: 3`) and aggregates pass@1, mean tokens, mean turns, p50 / p95 latency
- `Report.Print(os.Stdout)` emits a tab-aligned summary; `Report.WriteCSV(w)` is also available for archiving across runs

## Composing scorers

| Scorer | When to use |
|---|---|
| `Contains{Substr}` | Quick smoke checks, factual one-liners |
| `Equal{Want}` | Exact-match short answers (trims whitespace) |
| `Regex{Pattern}` | Numeric answers, structured output formats |
| `JSONField{FieldPath, Want}` | Structured JSON output where one field is the verdict |
| `LLMJudge{Client, Model, Rubric}` | Subjective tasks (creative writing, "is this answer reasonable") |
| `AllOf{...}` | Multiple criteria must all hold |
| `AnyOf{...}` | One of several valid answers |
| `Not{Scorer}` | Inversion, e.g. "must NOT mention this term" |

## Reproducibility

`LLMJudge` is non-deterministic. Pin `Temperature: 0` and the same judge model across runs; budget for occasional flakes. The `judge_confidence` and `judge_tokens` notes on each `Score` give you a signal when verdicts are borderline.

For deterministic scoring, prefer the non-LLM scorers.
