# agents/eval

Score [`agents.RunResult`](..) values produced by `agents.Runner`. Aggregate pass@1, mean tokens, mean turns, and latency percentiles across many tasks.

```go
import "github.com/buckedunicorn/grok/agents/eval"
```

## Surface

| Type / function | Role |
|---|---|
| `Scorer` | Interface: `Score(ctx, RunResult) (Verdict, error)` |
| `Contains`, `Equal`, `Regex`, `JSONField` | Substring, exact, regex, and JSON-field scorers |
| `AllOf`, `AnyOf`, `Not` | Boolean combinators |
| `LLMJudge` | LLM-as-judge scorer for open-ended outputs |
| `Suite` | Holds a list of `Task` values and a concurrency cap. `Run` executes them and returns a `Report` |
| `Report` | Aggregated metrics. `Print` writes a summary; `WriteCSV` exports per-task rows |

## Example

```go
suite := &eval.Suite{
    Concurrency: 4,
    Tasks: []eval.Task{
        {Name: "fr-translation", Input: "Translate 'hello' to French.", Scorer: eval.Contains("bonjour")},
        {Name: "es-translation", Input: "Translate 'hello' to Spanish.", Scorer: eval.Contains("hola")},
    },
}

report, err := suite.Run(ctx, runner, agent)
if err != nil { return err }

report.Print(os.Stdout)
_ = report.WriteCSV("eval-results.csv")
```

## Examples directory

- [`examples/eval`](../../examples/eval)
- [`examples/sandboxed-eval`](../../examples/sandboxed-eval)
