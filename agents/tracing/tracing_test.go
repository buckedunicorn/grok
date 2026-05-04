package tracing_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/agents/tracing"
	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/internal/transport"
)

// recorder builds a fresh in-memory tracer + exporter pair. Returns the
// provider (callers can grab .Tracer(...)) and the exporter, which
// .GetSpans() lists every span the tracer ever emitted.
func recorder() (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	return tp, exp
}

func chatStub(t *testing.T, completions []chat.Completion) (*chat.Client, func()) {
	t.Helper()
	idx := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if idx >= len(completions) {
			t.Errorf("chatStub exhausted at call #%d", idx+1)
			http.Error(w, "exhausted", 500)
			return
		}
		json.NewEncoder(w).Encode(completions[idx])
		idx++
	}))
	tr := transport.NewInsecure("test-key", srv.URL, srv.Client())
	return chat.NewClient(tr), srv.Close
}

func text(content string) chat.Completion {
	return chat.Completion{
		Choices: []chat.Choice{{
			FinishReason: "stop",
			Message:      chat.Message{Role: "assistant", Content: content},
		}},
		Usage: chat.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
}

func toolCall(id, name, args string) chat.Completion {
	return chat.Completion{
		Choices: []chat.Choice{{
			FinishReason: "tool_calls",
			Message: chat.Message{
				Role: "assistant",
				ToolCalls: []chat.ToolCall{{
					ID:       id,
					Type:     "function",
					Function: chat.FunctionCallData{Name: name, Arguments: args},
				}},
			},
		}},
	}
}

func TestHooks_emitsTurnAndToolSpans(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{text("ok")})
	defer cleanup()

	tp, exp := recorder()
	tracer := tp.Tracer("test")

	r := &agents.Runner{Client: c, Hooks: tracing.Hooks(tracer)}
	if _, err := r.Run(context.Background(), &agents.Agent{Name: "X", Model: "m"}, agents.RunOptions{Input: "hi"}); err != nil {
		t.Fatal(err)
	}

	spans := exp.GetSpans()
	// Expect at least: turn start (agent.turn), turn end (agent.turn.end).
	names := spanNames(spans)
	if !contains(names, "agent.turn") {
		t.Errorf("missing agent.turn span, got %v", names)
	}
	if !contains(names, "agent.turn.end") {
		t.Errorf("missing agent.turn.end span, got %v", names)
	}
}

func TestHooks_recordsUsageOnTurnEnd(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{text("ok")})
	defer cleanup()

	tp, exp := recorder()
	tracer := tp.Tracer("test")

	r := &agents.Runner{Client: c, Hooks: tracing.Hooks(tracer)}
	if _, err := r.Run(context.Background(), &agents.Agent{Name: "X", Model: "m"}, agents.RunOptions{Input: "hi"}); err != nil {
		t.Fatal(err)
	}

	spans := exp.GetSpans()
	end := findSpan(spans, "agent.turn.end")
	if end == nil {
		t.Fatal("agent.turn.end span not found")
	}
	if got := attrInt(end, "usage.total_tokens"); got != 15 {
		t.Errorf("usage.total_tokens = %d, want 15", got)
	}
	if got := attrString(end, "finish_reason"); got != "stop" {
		t.Errorf("finish_reason = %q, want stop", got)
	}
}

func TestHooks_emitsToolCallAndResultSpans(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{
		toolCall("c1", "echo", `{"x":1}`),
		text("done"),
	})
	defer cleanup()

	tp, exp := recorder()
	tracer := tp.Tracer("test")

	tool := agents.Tool{
		Name:    "echo",
		Handler: func(_ context.Context, args string) (string, error) { return args, nil },
	}
	r := &agents.Runner{Client: c, Hooks: tracing.Hooks(tracer)}
	if _, err := r.Run(context.Background(),
		&agents.Agent{Name: "X", Model: "m", Tools: []agents.Tool{tool}},
		agents.RunOptions{Input: "go"},
	); err != nil {
		t.Fatal(err)
	}

	spans := exp.GetSpans()
	names := spanNames(spans)
	if !contains(names, "agent.tool_call") {
		t.Errorf("missing agent.tool_call, got %v", names)
	}
	if !contains(names, "agent.tool_call.result") {
		t.Errorf("missing agent.tool_call.result, got %v", names)
	}
	call := findSpan(spans, "agent.tool_call")
	if attrString(call, "tool.name") != "echo" {
		t.Errorf("tool.name = %q", attrString(call, "tool.name"))
	}
}

func TestHooks_toolErrorRecorded(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{
		toolCall("c1", "boom", `{}`),
		text("done"),
	})
	defer cleanup()

	tp, exp := recorder()
	tracer := tp.Tracer("test")

	tool := agents.Tool{
		Name:    "boom",
		Handler: func(_ context.Context, _ string) (string, error) { return "", errors.New("kaboom") },
	}
	r := &agents.Runner{Client: c, Hooks: tracing.Hooks(tracer)}
	if _, err := r.Run(context.Background(),
		&agents.Agent{Name: "X", Model: "m", Tools: []agents.Tool{tool}},
		agents.RunOptions{Input: "go"},
	); err != nil {
		t.Fatal(err)
	}

	spans := exp.GetSpans()
	res := findSpan(spans, "agent.tool_call.result")
	if res == nil {
		t.Fatal("missing tool_call.result span")
	}
	// Span status code should be Error.
	if !strings.Contains(res.Status.Description, "kaboom") {
		t.Errorf("expected error description to contain 'kaboom', got %q", res.Status.Description)
	}
}

func TestHooks_handoffEmitsSpan(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{
		toolCall("c1", "to_specialist", `{"reason":"x"}`),
		text("specialist answer"),
	})
	defer cleanup()

	tp, exp := recorder()
	tracer := tp.Tracer("test")

	specialist := &agents.Agent{Name: "Specialist", Model: "m"}
	root := &agents.Agent{
		Name:  "Root",
		Model: "m",
		Handoffs: []agents.Handoff{{
			Name:        "to_specialist",
			Description: "Hand off",
			Agent:       specialist,
		}},
	}

	r := &agents.Runner{Client: c, Hooks: tracing.Hooks(tracer)}
	if _, err := r.Run(context.Background(), root, agents.RunOptions{Input: "go"}); err != nil {
		t.Fatal(err)
	}

	spans := exp.GetSpans()
	h := findSpan(spans, "agent.handoff")
	if h == nil {
		t.Fatal("missing agent.handoff span")
	}
	if attrString(h, "handoff.from") != "Root" || attrString(h, "handoff.to") != "Specialist" {
		t.Errorf("handoff attrs wrong: %+v", h.Attributes)
	}
}

func TestMiddleware_wrapsRunInParentSpan(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{text("hello")})
	defer cleanup()

	tp, exp := recorder()
	tracer := tp.Tracer("test")

	r := &agents.Runner{
		Client:     c,
		Hooks:      tracing.Hooks(tracer),
		Middleware: []agents.Middleware{tracing.Middleware(tracer)},
	}
	res, err := r.Run(context.Background(), &agents.Agent{Name: "X", Model: "m"}, agents.RunOptions{Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}

	spans := exp.GetSpans()
	run := findSpan(spans, "agent.run")
	if run == nil {
		t.Fatal("missing agent.run parent span")
	}
	if attrString(run, "agent.last_agent") != "X" {
		t.Errorf("last_agent = %q", attrString(run, "agent.last_agent"))
	}
	if attrString(run, "agent.run_id") != res.Trajectory.RunID {
		t.Errorf("run_id mismatch: span=%q result=%q",
			attrString(run, "agent.run_id"), res.Trajectory.RunID)
	}
}

func TestHooks_nilTracerNoOps(t *testing.T) {
	hooks := tracing.Hooks(nil)
	if hooks.OnTurnStart != nil || hooks.OnTurnEnd != nil ||
		hooks.OnToolCall != nil || hooks.OnToolResult != nil || hooks.OnHandoff != nil {
		t.Error("nil tracer should produce zero-value RunHooks")
	}
}

func TestMiddleware_nilTracerPassthrough(t *testing.T) {
	mw := tracing.Middleware(nil)
	called := false
	wrapped := mw(func(_ context.Context, _ *agents.Agent, _ agents.RunOptions) (*agents.RunResult, error) {
		called = true
		return &agents.RunResult{}, nil
	})
	_, err := wrapped(context.Background(), &agents.Agent{}, agents.RunOptions{})
	if err != nil || !called {
		t.Errorf("nil tracer middleware should pass through unchanged")
	}
}

// --- helpers ---

func spanNames(spans tracetest.SpanStubs) []string {
	out := make([]string, len(spans))
	for i, s := range spans {
		out[i] = s.Name
	}
	return out
}

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}

func findSpan(spans tracetest.SpanStubs, name string) *tracetest.SpanStub {
	for i := range spans {
		if spans[i].Name == name {
			return &spans[i]
		}
	}
	return nil
}

func attrString(s *tracetest.SpanStub, key string) string {
	for _, a := range s.Attributes {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	return ""
}

func attrInt(s *tracetest.SpanStub, key string) int64 {
	for _, a := range s.Attributes {
		if string(a.Key) == key {
			return a.Value.AsInt64()
		}
	}
	return 0
}
