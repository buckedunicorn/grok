# runnable

A generic `Runnable[In, Out]` interface and small combinators (`Pipe`, `Parallel`, `Branch`, `Map`, `Retry`) for composing typed pipelines. Stdlib-only.

```go
import "github.com/buckedunicorn/grok/runnable"
```

## When to use

`runnable` is the SDK's universal connector. Wrap any function with `runnable.Func[In, Out]` and stitch together prompts, chat calls, parsers, post-processing, and retries into one typed pipeline.

It is opt-in sugar. Calling `chat.Client` directly stays the simplest path for one-off requests.

## Surface

| Type / function | Purpose |
|---|---|
| `Runnable[In, Out]` | Interface: `Invoke(ctx, In) (Out, error)` |
| `Func[In, Out](fn)` | Wraps a plain function as a `Runnable` |
| `Pipe`, `Pipe3`, `Pipe4` | Sequential composition |
| `Parallel` | Concurrent execution; collects results |
| `Branch` | Conditional dispatch |
| `Map` | Element-wise; fan-out over a slice |
| `ParallelMap` | Concurrent fan-out |
| `Retry` | Re-invoke on error with backoff |

## Example

```go
type req struct{ Phrase, Language string }

renderPrompt := runnable.Func[req, []chat.Message](func(ctx context.Context, in req) ([]chat.Message, error) {
    return []chat.Message{{Role: "user", Content: fmt.Sprintf("Translate '%s' to %s.", in.Phrase, in.Language)}}, nil
})
callModel := runnable.Func[[]chat.Message, *chat.Completion](func(ctx context.Context, msgs []chat.Message) (*chat.Completion, error) {
    return client.Chat.Create(ctx, &chat.CreateRequest{Model: "grok-4-1-fast-non-reasoning", Messages: msgs})
})
extractText := runnable.Func[*chat.Completion, string](func(_ context.Context, comp *chat.Completion) (string, error) {
    s, _ := comp.Choices[0].Message.Content.(string)
    return s, nil
})

pipeline := runnable.Pipe3(renderPrompt, callModel, extractText)
out, _ := pipeline.Invoke(ctx, req{Phrase: "hello", Language: "French"})
```

## Examples directory

- [`examples/sugar`](../examples/sugar) (Pipe3 pipeline)
- [`examples/workflow`](../examples/workflow)
