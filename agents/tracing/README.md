# agents/tracing

OpenTelemetry instrumentation for [`agents.Runner`](..). Drop the returned hooks and middleware onto a Runner and every `Run` call becomes a hierarchy of spans.

```go
import "github.com/buckedunicorn/grok/agents/tracing"
```

## Surface

| Function | Purpose |
|---|---|
| `Hooks(tracer)` | Returns an `agents.RunHooks` value whose lifecycle callbacks emit per-turn, per-tool, and per-handoff spans |
| `Middleware(tracer)` | Wraps each `Run` call in a parent `agent.run` span |

## Example

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"

    "github.com/buckedunicorn/grok/agents"
    "github.com/buckedunicorn/grok/agents/tracing"
)

exporter, _ := stdouttrace.New(stdouttrace.WithPrettyPrint())
tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
otel.SetTracerProvider(tp)
tracer := tp.Tracer("github.com/your/app")

runner := agents.NewRunner(client.Chat,
    agents.WithHooks(tracing.Hooks(tracer)),
    agents.WithMiddleware(tracing.Middleware(tracer)),
)
```

The package depends on `go.opentelemetry.io/otel` directly. Bring your own exporter (`stdouttrace`, OTLP, vendor-specific) and tracer provider; the package only consumes the `trace.Tracer` interface.

## Examples directory

- [`examples/tracing`](../../examples/tracing)
- [`examples/full-stack`](../../examples/full-stack)
