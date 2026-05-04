# tracing

OpenTelemetry instrumentation for `agents.Runner`.

## Run

```sh
XAI_API_KEY=... go run ./examples/tracing
```

Spans are written to stdout as pretty-printed JSON. For production use, swap the `stdouttrace` exporter for any OTLP exporter (`otlptracegrpc`, `otlptracehttp`, vendor-specific, etc.), the rest of the wiring is identical.

## What it shows

- `tracing.Hooks(tracer)` returns an `agents.RunHooks` value that emits spans on every lifecycle event: turn start/end, tool call/result, handoff
- `tracing.Middleware(tracer)` returns an `agents.Middleware` that wraps each `Run` call in a single parent `agent.run` span, total run latency, output size, error status, and run id all become attributes
- Both can be composed: hooks for fine-grained turn/tool spans, middleware for the run-level parent. They share the same trace.

## Span hierarchy

```
agent.run                (from Middleware, once per Run call)
├─ agent.turn            (from Hooks.OnTurnStart, once per loop turn)
├─ agent.turn.end        (from Hooks.OnTurnEnd, usage, finish_reason)
├─ agent.tool_call       (from Hooks.OnToolCall, name, args size)
├─ agent.tool_call.result (from Hooks.OnToolResult, result size, error)
└─ agent.handoff         (from Hooks.OnHandoff, from/to/reason)
```

The hooks emit sibling spans rather than nested children because `RunHooks` doesn't return a context, threading parent-child between sibling callbacks would require stateful tracking. Sibling spans plus the `agent.turn` attribute give the same observability without the complexity. If you want strict parent-child between a tool call and its result, add the `Middleware` form: every span lands inside the run's span.

## Choosing your exporter

The example uses `stdouttrace.WithPrettyPrint()` for legibility. Real exporters:

| Backend | Package |
|---|---|
| Jaeger / Tempo / OTel Collector (gRPC) | `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` |
| Same backends over HTTP | `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` |
| Datadog | `github.com/DataDog/dd-trace-go` (Datadog's own otel adapter) |
| Honeycomb | OTLP gRPC pointed at honeycomb's endpoint with auth headers |

The grok SDK only depends on the core OTel API + SDK packages, so you pick the exporter at app level.
