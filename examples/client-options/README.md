# client-options

Demonstrates `WithLogger`, `WithMaxRetries`, and `WithConcurrency` working together.

## Run

```sh
XAI_API_KEY=... go run ./examples/client-options
```

## What it shows

- `grok.WithLogger`: wires a `log/slog.Logger` into the transport layer; retry attempts and non-retryable errors appear as structured log lines
- `grok.WithMaxRetries`: how many times the transport retries `429 Too Many Requests` and `5xx` responses before giving up
- `grok.WithConcurrency`: a semaphore that caps in-flight HTTP requests regardless of how many goroutines call the client concurrently, useful for batch workloads where you spawn many goroutines but want to stay within rate-limit budgets
