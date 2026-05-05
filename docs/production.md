# Production guide

Settings and patterns that matter once you are past the prototype phase. None of this is required for a hello-world; all of it bites in production.

## Retries and rate limits

The xAI API surfaces transient failures as `429 Too Many Requests` and the standard `5xx` family. The SDK can retry these for you with exponential backoff and jitter:

```go
client := grok.New(
    grok.WithAPIKey(os.Getenv("XAI_API_KEY")),
    grok.WithMaxRetries(3),
)
```

Behaviour:

- Retries trigger on `429`, `500`, `502`, `503`, `504`, and on `net/http` errors that are not the request's context cancellation.
- The retry layer honours `Retry-After` headers when present.
- Backoff is exponential with full jitter, capped per-request.
- The request's `context.Context` is honoured: a deadline that has already passed prevents retries, and cancellation is observed during the backoff sleep.

If you need more control (per-endpoint retry classes, custom budgets, hedging), wrap your sub-client calls yourself and leave `WithMaxRetries` at zero. The transport's retry layer is intentionally simple.

### Concurrency cap

`grok.WithConcurrency(n)` limits in-flight requests *across all sub-clients of one Client*. Useful when you have a worker pool that can outpace the API's per-key limits and you would rather queue locally than trip 429.

```go
client := grok.New(
    grok.WithAPIKey(os.Getenv("XAI_API_KEY")),
    grok.WithMaxRetries(3),
    grok.WithConcurrency(8),
)
```

For finer-grained shaping (per-tenant fairness, token-bucket smoothing) reach for the [`queue`](../queue) package and wrap the `*Client` calls inside `queue.Queue.Submit`.

## Prompt caching

The xAI API caches request prefixes server-side. Cache hits are billed at a discount and reduce latency. To maximise hit rate, pin a `x-grok-conv-id` to all turns of one logical conversation:

```go
client := grok.New(
    grok.WithAPIKey(os.Getenv("XAI_API_KEY")),
    grok.WithConvID("conversation-" + sessionID),
)
```

Or per-conversation:

```go
sessionClient := client.WithConvID("conversation-" + sessionID)
```

Read the cache hit count from `Usage.PromptTokensDetails.CachedTokens` (chat) or `Usage.InputTokensDetails.CachedTokens` (Responses). For best hit rates, keep the system prompt and tool definitions stable across turns, and prepend new content rather than rewriting earlier turns.

The [`cache`](../cache) package wraps this with a couple of conveniences but adds nothing beyond the header.

## Sandboxing tool execution

Anything that lets a model run code, touch the filesystem, or hit the network is part of your trust boundary. Two pieces of the SDK help:

- [`agents/harness`](../agents/harness) provides an opinionated default agent with planning, FS, shell, and subagent tools. The default `Executor` (`LocalExecutor`) runs commands on the host with no isolation — fine for local-developer use, dangerous for anything that handles untrusted input.
- [`agents/sandbox`](../agents/sandbox) provides `DockerExecutor`, which shells out to `docker run --rm` per command with hardening defaults: `network=none`, `cap-drop=ALL`, `no-new-privileges`, read-only root with a `/tmp` tmpfs, `pids-limit=256`, `user=65534:65534` (nobody). Override per-deployment via `NetworkOn`, `AllowSetUID`, `WritableRoot`, `Caps`, `PIDsLimit`, `User`.

```go
exec := &sandbox.DockerExecutor{
    Image:    "alpine:3.20",
    MemoryMB: 256,
    CPUs:     1.0,
}
h := harness.NewDefault(client.Chat, "grok-4-1-fast-reasoning",
    harness.WithExecutor(exec),
    harness.WithMaxTurns(10),
)
```

For filesystem isolation, prefer `harness.LocalFS{Root: dir}` over the default `MemoryFS`; `LocalFS` rejects path traversal and refuses to follow symlinks out of `Root`.

## Observability

The SDK exposes three layers of observability:

### Logging

`grok.WithLogger(slog)` attaches a structured logger that emits one event per request and one per retry. Use it as the cheap baseline:

```go
logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
client := grok.New(grok.WithLogger(logger), grok.WithMaxRetries(3))
```

### Tracing for the agent loop

The [`agents/tracing`](../agents/tracing) package adapts `agents.Runner` to OpenTelemetry. Drop its `Hooks` and `Middleware` onto a Runner and every `Run` call becomes a hierarchy of spans an OTel-aware backend (Jaeger, Tempo, Honeycomb, Datadog) can index:

```go
tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
defer tp.Shutdown(ctx)
otel.SetTracerProvider(tp)
tracer := tp.Tracer("github.com/your-org/your-app")

runner := &agents.Runner{
    Client:     client.Chat,
    Hooks:      tracing.Hooks(tracer),
    Middleware: []agents.Middleware{tracing.Middleware(tracer)},
}
```

The package emits per-turn, per-tool, and per-handoff spans with usage, finish reason, and tool-result attributes.

### Trajectory persistence

For offline analysis, replay, and eval, the `agents.Runner` always returns a structured `Trajectory` on the `RunResult`. Persist it directly (it is JSON-serialisable) or feed it to [`agents/eval`](../agents/eval) for pass@1, latency percentiles, and per-task scoring.

The [`agents/durable`](../agents/durable) package adds checkpoint persistence: every Runner turn is saved to a `Store`, and `RunOptions.Resume` picks up at the saved `NextTurn` with the trajectory and session intact.

## Cost tracking

Every response carries a `Usage` struct with token counts and `CostInUSDTicks` in micro-cents (10^-6 USD; divide by 1e8 for dollars). Track it across every call — even small variances in cache hit rate or reasoning effort move the total significantly.

For the Responses API with server-side tools (`web_search`, `x_search`, `code_interpreter`), `NumServerSideToolsUsed` reports how many billed server-tool invocations the model made on top of the model tokens.

## Testing

Integration tests in this repo skip when `XAI_API_KEY` is unset. For your own tests:

- Replace the HTTP client via `grok.WithHTTPClient(fake)` to avoid the network entirely.
- Or set `grok.WithBaseURL("http://127.0.0.1:N")` paired with `grok.WithInsecureBaseURL()` and run a `httptest.Server` recording mock; this keeps the SDK's encoding/decoding paths in the test.
- For agent tests, `agents.Runner` accepts any `*chat.Client`, so a fake `chat.Client` covers the runner contract.

Set `WithMaxRetries(0)` in tests so retry storms do not mask flakes.

## Versioning and supported branches

Security fixes ship against `main` and the most recent tagged minor version; see [`SECURITY.md`](../SECURITY.md). Tagged releases follow semver. Pin to a specific tag in production (`go get github.com/buckedunicorn/grok@vX.Y.Z`); `go mod tidy` will keep the indirect dep tree consistent.

## See also

- [`docs/getting-started.md`](./getting-started.md), [`docs/configuration.md`](./configuration.md), [`docs/streaming-and-tools.md`](./streaming-and-tools.md).
- [`SECURITY.md`](../SECURITY.md) for the disclosure policy and threat model.
- [`agents`](../agents) and its sub-packages for the agent-runtime layer.
