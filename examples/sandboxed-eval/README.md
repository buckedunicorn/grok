# sandboxed-eval

Composes `agents/eval` with a sandboxed `harness`: every task runs through `harness.NewDefault` wired to a `sandbox.DockerExecutor`, and `eval.Suite` aggregates pass-rate and resource use across the runs.

## Run

```sh
docker pull python:3.12-alpine
XAI_API_KEY=... go run ./examples/sandboxed-eval
```

## What it shows

- **Per-task isolated workspace.** Each `runFn` invocation creates a fresh `os.MkdirTemp` workspace, a `LocalFS` rooted at it, and a `DockerExecutor` mounting it. No state leaks between tasks.
- **eval-style reporting over real code execution.** Tasks write Python, run it inside a container, and reply with the output. Scorers (`Contains`, `AllOf`, `AnyOf`) check the output against expected fragments. The final report prints pass@1, mean tokens, mean turns, and p50/p95 latency.
- **Layered scorers.** `eval.AllOf{Contains{"True"}, Contains{"False"}}` requires both fragments, useful when one expected substring isn't enough to distinguish a correct answer from a hallucination.

## Tasks

| ID | What | Scorer |
|---|---|---|
| primes-20 | first 20 primes on one line | both `2 3 5 7` and `67 71` |
| fizzbuzz-15 | FizzBuzz n=1..15 | both `FizzBuzz` and `Fizz\n4\nBuzz` |
| factorial-10 | 10! recursively | `3628800` |
| palindrome | three palindrome checks | both `True` and `False` |

## Concurrency

`Suite.Concurrency: 1` runs sequentially. Raise it to fan out, every parallel run spawns its own ephemeral container, so a beefy machine can handle several at once. Watch for `docker pull` contention if the image isn't cached: pre-pull before raising concurrency.

## Extending

Replace the `Expect` field with `eval.LLMJudge` to grade subjective outputs (e.g. "the agent explained its reasoning clearly"). Or add a custom `eval.ScorerFunc` that re-executes the agent's `solution.py` against a hidden test suite.
