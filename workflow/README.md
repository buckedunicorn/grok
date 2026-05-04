# workflow

Composable building blocks for multi-step AI pipelines. Stdlib-only.

```go
import "github.com/buckedunicorn/grok/workflow"
```

## When to use

`workflow` is a higher-level alternative to [`runnable`](../runnable) when every step in your pipeline is a `string -> string` transform (the common case for chat-completion chains). It trades the type-parameter generality of `runnable` for terser composition.

For mixed-type pipelines, prefer `runnable`.

## Surface

| Type / function | Purpose |
|---|---|
| `Step` | A function that transforms a string input into a string output |
| `Chain(steps...)` | Sequential composition |
| `Parallel(steps...)` | Concurrent execution; the combiner reduces results to a single string |
| `Conditional(predicate, ifTrue, ifFalse)` | Branch on the input |

## Example

```go
extract := workflow.Step(func(ctx context.Context, in string) (string, error) {
    comp, _ := client.Chat.Create(ctx, &chat.CreateRequest{
        Model:    "grok-4-1-fast-non-reasoning",
        Messages: []chat.Message{{Role: "user", Content: "Extract the main topic of: " + in}},
    })
    s, _ := comp.Choices[0].Message.Content.(string)
    return s, nil
})

summarise := workflow.Step(func(ctx context.Context, topic string) (string, error) {
    // ...
    return "summary of " + topic, nil
})

pipeline := workflow.Chain(extract, summarise)
out, err := pipeline.Run(ctx, longArticle)
```

## Examples directory

- [`examples/workflow`](../examples/workflow)
