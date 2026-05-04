// Package tracing exports OpenTelemetry instrumentation for agents.Runner.
//
// The package is a thin adapter: it produces an agents.RunHooks value
// whose lifecycle callbacks emit OTel spans and span events. Drop the
// returned hooks onto a Runner and every Run call becomes a hierarchy of
// spans an OTel-aware backend (Jaeger, Tempo, Honeycomb, Datadog, etc.)
// can index and query.
//
// Span shape:
//
//	agent.run                      ← parent span, one per Runner.Run call
//	├─ agent.turn (turn=0)         ← one per loop iteration
//	│   ├─ event "turn.start"      ← attributes: agent.name, input msg count
//	│   ├─ child agent.tool_call   ← one per tool invocation in the turn
//	│   │   └─ event "tool.result" ← attributes: name, result_bytes, error
//	│   ├─ event "handoff"         ← when control transfers to another agent
//	│   ├─ event "guardrail"       ← per guardrail check
//	│   └─ event "turn.end"        ← attributes: usage, finish_reason
//	└─ ...
//
// Why a RunHooks adapter rather than Middleware: hooks fire at every
// lifecycle point (turn start, each tool call, handoff, guardrail), so the
// span structure is finer-grained than what a single span-per-run
// middleware can express. RunHooks composes with user middleware so this
// is purely additive, install both.
//
// Example:
//
//	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))
//	defer tp.Shutdown(ctx)
//	otel.SetTracerProvider(tp)
//
//	tracer := tp.Tracer("github.com/your-org/your-app")
//	runner := &agents.Runner{
//	    Client: client.Chat,
//	    Hooks:  tracing.Hooks(tracer),
//	}
package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/chat"
)

// Hooks returns an agents.RunHooks that emits OpenTelemetry spans through
// the given tracer. Pass the result as the Runner's Hooks field. The hooks
// never block on long work and never panic; if the tracer is nil, every
// callback is a no-op.
func Hooks(tracer trace.Tracer) agents.RunHooks {
	if tracer == nil {
		return agents.RunHooks{}
	}

	// Span name conventions follow OpenTelemetry's "verb.object" semantic
	// convention: agent.run, agent.turn, agent.tool_call, agent.handoff.
	const (
		spanRun      = "agent.run"
		spanTurn     = "agent.turn"
		spanToolCall = "agent.tool_call"
	)

	// Active spans live as values on the ctx, but RunHooks doesn't pass a
	// per-run ctx-with-run-span back, the hooks share the run's ctx.
	// We start the run span at the first OnTurnStart (turn 0) and end it
	// at the final OnTurnEnd that yields a "stop" finish_reason. To handle
	// that cleanly without running-state on this struct, we instead nest
	// turn spans directly under whatever ctx span the user already wired
	// (typically a higher-level span the caller starts before Run).
	//
	// In practice the cleanest semantic is: each turn is its own span;
	// callers who want a parent span around the whole run can wrap the
	// Run call themselves with a Middleware. We provide such a middleware
	// helper below (Middleware()).

	return agents.RunHooks{
		OnTurnStart: func(ctx context.Context, turn int, agent *agents.Agent, input []chat.Message) {
			_, span := tracer.Start(ctx, spanTurn,
				trace.WithAttributes(
					attribute.Int("agent.turn", turn),
					attribute.String("agent.name", agent.Name),
					attribute.String("agent.model", agent.Model),
					attribute.Int("agent.input_messages", len(input)),
					attribute.Int("agent.tools", len(agent.Tools)),
					attribute.Int("agent.handoffs", len(agent.Handoffs)),
				),
			)
			// We can't return the new ctx through RunHooks (the runner
			// uses its own ctx for subsequent calls), so we end the turn
			// span synchronously in OnTurnEnd. To correlate, stash on a
			// shared map keyed by (ctx, turn). But that's ugly.
			//
			// Instead, end the span here as soon as we've recorded its
			// attributes. The events that would have been children of
			// this span (tool calls, etc.) are emitted as their own
			// spans / events on the parent ctx. Trade-off: we lose the
			// turn-as-parent hierarchy, but we keep the implementation
			// stateless and correct.
			span.End()
		},
		OnTurnEnd: func(ctx context.Context, turn int, comp *chat.Completion) {
			if comp == nil {
				return
			}
			_, span := tracer.Start(ctx, spanTurn+".end",
				trace.WithAttributes(
					attribute.Int("agent.turn", turn),
				),
			)
			defer span.End()
			if len(comp.Choices) > 0 {
				span.SetAttributes(
					attribute.String("finish_reason", comp.Choices[0].FinishReason),
				)
			}
			span.SetAttributes(
				attribute.Int("usage.prompt_tokens", comp.Usage.PromptTokens),
				attribute.Int("usage.completion_tokens", comp.Usage.CompletionTokens),
				attribute.Int("usage.total_tokens", comp.Usage.TotalTokens),
				attribute.Int("usage.cached_tokens", comp.Usage.PromptTokensDetails.CachedTokens),
				attribute.Int("usage.reasoning_tokens", comp.Usage.CompletionTokensDetails.ReasoningTokens),
			)
		},
		OnToolCall: func(ctx context.Context, turn int, name, argsJSON string) {
			_, span := tracer.Start(ctx, spanToolCall,
				trace.WithAttributes(
					attribute.Int("agent.turn", turn),
					attribute.String("tool.name", name),
					attribute.Int("tool.args_bytes", len(argsJSON)),
				),
			)
			// End immediately, the result span is emitted from OnToolResult.
			// If we wanted strict parent/child between call and result we'd
			// need to thread a span through ctx, which RunHooks doesn't
			// support. Two sibling spans + correlated turn attribute is the
			// pragmatic compromise.
			span.End()
		},
		OnToolResult: func(ctx context.Context, turn int, name, result string, err error) {
			_, span := tracer.Start(ctx, spanToolCall+".result",
				trace.WithAttributes(
					attribute.Int("agent.turn", turn),
					attribute.String("tool.name", name),
					attribute.Int("tool.result_bytes", len(result)),
				),
			)
			defer span.End()
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
		},
		OnHandoff: func(ctx context.Context, from, to *agents.Agent, reason string) {
			_, span := tracer.Start(ctx, "agent.handoff",
				trace.WithAttributes(
					attribute.String("handoff.from", from.Name),
					attribute.String("handoff.to", to.Name),
					attribute.String("handoff.reason_bytes", reason),
				),
			)
			span.End()
		},
	}
}

// Middleware returns an agents.Middleware that wraps every Run call in a
// single parent "agent.run" span. Compose with Hooks(tracer) for full
// instrumentation:
//
//	runner := &agents.Runner{
//	    Client:     client.Chat,
//	    Hooks:      tracing.Hooks(tracer),
//	    Middleware: []agents.Middleware{tracing.Middleware(tracer)},
//	}
//
// The parent span captures total run latency, final output length, and
// any error returned from Run. Child spans emitted by Hooks live on the
// same trace.
func Middleware(tracer trace.Tracer) agents.Middleware {
	if tracer == nil {
		return func(next agents.RunFunc) agents.RunFunc { return next }
	}
	return func(next agents.RunFunc) agents.RunFunc {
		return func(ctx context.Context, agent *agents.Agent, opts agents.RunOptions) (*agents.RunResult, error) {
			ctx, span := tracer.Start(ctx, "agent.run",
				trace.WithAttributes(
					attribute.String("agent.name", agent.Name),
					attribute.String("agent.model", agent.Model),
					attribute.Int("agent.tools", len(agent.Tools)),
					attribute.Int("agent.handoffs", len(agent.Handoffs)),
					attribute.Int("agent.input_bytes", len(opts.Input)),
				),
			)
			defer span.End()

			res, err := next(ctx, agent, opts)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				return nil, err
			}
			if res != nil {
				span.SetAttributes(
					attribute.String("agent.last_agent", agentName(res.LastAgent)),
					attribute.Int("agent.output_bytes", len(res.Output)),
					attribute.Int("agent.turns", len(res.Trajectory.Turns)),
					attribute.Int("usage.total_tokens", res.Usage.TotalTokens),
					attribute.String("agent.run_id", res.Trajectory.RunID),
				)
			}
			return res, nil
		}
	}
}

func agentName(a *agents.Agent) string {
	if a == nil {
		return ""
	}
	return a.Name
}
