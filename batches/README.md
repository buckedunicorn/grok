# batches

`/v1/batches` endpoints. Asynchronous bulk inference at reduced cost (typically 20-50% off standard rates) with completion within 24 hours.

```go
import "github.com/buckedunicorn/grok/batches"
```

## When to use

Reach for batches when you have a large queue of independent chat completion requests and your latency budget is hours, not seconds. Examples: offline data labeling, document classification, large-scale evals.

## Surface

| API | Purpose |
|---|---|
| `Client.Create` | Create an empty batch |
| `Client.AddRequests` | Append one or more chat-completion requests to a batch |
| `Client.Get`, `Client.List` | Inspect batches |
| `Client.GetResults` | Fetch completed results |
| `Client.Wait` | Polls until the batch reaches a terminal state |
| `Client.Cancel` | Cancel an in-flight batch |
| `Client.All`, `Client.AllRequests`, `Client.AllResults` | Iterator helpers (`iter.Seq2`) |

## Example

```go
b, err := client.Batches.Create(ctx, "nightly-eval")
if err != nil { return err }

err = client.Batches.AddRequests(ctx, b.ID, []batches.BatchRequest{
    {ChatGetCompletion: &chat.CreateRequest{
        Model: "grok-4-1-fast-non-reasoning",
        Messages: []chat.Message{{Role: "user", Content: "..."}},
    }},
})
if err != nil { return err }

done, err := client.Batches.Wait(ctx, b.ID, time.Minute)
if err != nil { return err }

for r, err := range client.Batches.AllResults(ctx, done.ID, nil) {
    if err != nil { return err }
    fmt.Println(r.Response.Choices[0].Message.Content)
}
```

## Examples directory

- [`examples/batches`](../examples/batches)
