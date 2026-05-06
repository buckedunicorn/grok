// tracing demonstrates agents/tracing, OpenTelemetry instrumentation that
// turns every Runner.Run call into a hierarchy of spans an OTel-aware
// backend (Jaeger, Tempo, Honeycomb, Datadog, etc.) can index and query.
//
// This example wires the stdout exporter so spans print to your terminal as
// JSON. Swap in any OTLP exporter for production use:
//
//	import "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
//	exp, _ := otlptracegrpc.New(ctx)
//	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/agents/tracing"
)

func main() {
	ctx := context.Background()

	// --- OTel setup: stdout exporter with pretty-printed JSON ---
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	tracer := tp.Tracer("github.com/buckedunicorn/grok/examples/tracing")

	// --- Wire the hooks + middleware onto the Runner ---
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	multiply := agents.Tool{
		Name:        "multiply",
		Description: "Multiplies two numbers.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"a": map[string]any{"type": "number"},
				"b": map[string]any{"type": "number"},
			},
			"required": []string{"a", "b"},
		},
		Handler: func(_ context.Context, args string) (string, error) {
			var p struct{ A, B float64 }
			json.Unmarshal([]byte(args), &p)
			return fmt.Sprintf("%g", p.A*p.B), nil
		},
	}

	runner := &agents.Runner{
		Client:     client.Chat,
		Hooks:      tracing.Hooks(tracer),
		Middleware: []agents.Middleware{tracing.Middleware(tracer)},
	}

	res, err := runner.Run(ctx, &agents.Agent{
		Name:         "Calculator",
		Instructions: "Use the multiply tool when given an arithmetic question.",
		Model:        "grok-4-1-fast-reasoning",
		Tools:        []agents.Tool{multiply},
	}, agents.RunOptions{Input: "What is 23 multiplied by 17?"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Spans flush on tp.Shutdown via the deferred call. Print the answer
	// here for a clean separation between agent output and trace output.
	fmt.Println("\n=== Answer ===")
	fmt.Println(res.Output)
}
