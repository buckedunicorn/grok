# queue

Rate-limiting request queue with `queue.New`.

## Run

```sh
XAI_API_KEY=... go run ./examples/queue
```

## What it shows

- `queue.New(rps)` creates a token-bucket queue capped at `rps` requests per second
- `q.Submit(ctx, fn)` schedules a function call; the queue sleeps between calls to stay within the limit
- Using the queue in a for loop to fan out multiple prompts without triggering 429 rate-limit errors
- Total elapsed time printed at the end to verify the rate limit was respected
