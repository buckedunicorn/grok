# queue

Token-bucket rate limiter for outgoing API calls. Stdlib-only.

```go
import "github.com/buckedunicorn/grok/queue"
```

## When to use

Use `queue.Queue` to throttle outgoing requests when you have many goroutines that all hit the same rate-limited endpoint. The queue blocks `Submit` calls until the bucket has room, so a burst of N goroutines settles into a steady stream at the configured rate.

## Surface

| API | Purpose |
|---|---|
| `New(rps)` | Construct a queue limited to `rps` requests per second |
| `Queue.Submit(ctx, fn)` | Block until the bucket has a token, then run `fn`. Honours `ctx` cancellation |
| `Queue.SubmitAll(ctx, fns)` | Submit a slice of functions; returns when all complete |

## Example

```go
q := queue.New(5) // 5 requests per second

var wg sync.WaitGroup
for _, prompt := range prompts {
    wg.Add(1)
    go func(p string) {
        defer wg.Done()
        _ = q.Submit(ctx, func(ctx context.Context) error {
            comp, err := client.Chat.Create(ctx, &chat.CreateRequest{
                Model:    "grok-4-1-fast-non-reasoning",
                Messages: []chat.Message{{Role: "user", Content: p}},
            })
            if err != nil { return err }
            fmt.Println(comp.Choices[0].Message.Content)
            return nil
        })
    }(prompt)
}
wg.Wait()
```

## Examples directory

- [`examples/queue`](../examples/queue)
