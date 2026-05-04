package workflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/workflow"
)

func upper(_ context.Context, s string) (string, error)   { return strings.ToUpper(s), nil }
func exclaim(_ context.Context, s string) (string, error) { return s + "!", nil }
func fail(_ context.Context, s string) (string, error)    { return "", errors.New("step failed") }

func TestChain_sequential(t *testing.T) {
	out, err := workflow.Chain(context.Background(), "hello", upper, exclaim)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "HELLO!" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestChain_stopsOnError(t *testing.T) {
	called := false
	after := func(_ context.Context, s string) (string, error) {
		called = true
		return s, nil
	}
	_, err := workflow.Chain(context.Background(), "hi", fail, after)
	if err == nil {
		t.Fatal("expected error")
	}
	if called {
		t.Error("step after error should not be called")
	}
}

func TestParallel_allSucceed(t *testing.T) {
	outs, err := workflow.Parallel(context.Background(), "x", upper, exclaim)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outs[0] != "X" {
		t.Errorf("step 0: want X, got %q", outs[0])
	}
	if outs[1] != "x!" {
		t.Errorf("step 1: want x!, got %q", outs[1])
	}
}

func TestParallel_propagatesError(t *testing.T) {
	_, err := workflow.Parallel(context.Background(), "x", upper, fail)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestConditional_thenBranch(t *testing.T) {
	step := workflow.Conditional(
		func(s string) bool { return s == "yes" },
		upper,
		exclaim,
	)
	out, err := step(context.Background(), "yes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "YES" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestConditional_elseBranch(t *testing.T) {
	step := workflow.Conditional(
		func(s string) bool { return s == "yes" },
		upper,
		exclaim,
	)
	out, _ := step(context.Background(), "no")
	if out != "no!" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestConditional_nilElse_passthrough(t *testing.T) {
	step := workflow.Conditional(func(s string) bool { return false }, upper, nil)
	out, _ := step(context.Background(), "hi")
	if out != "hi" {
		t.Errorf("nil else should pass through, got %q", out)
	}
}

func TestTransform(t *testing.T) {
	step := workflow.Transform(strings.TrimSpace)
	out, err := step(context.Background(), "  hello  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestRetry_succeedsEventually(t *testing.T) {
	attempts := 0
	flaky := func(_ context.Context, s string) (string, error) {
		attempts++
		if attempts < 3 {
			return "", errors.New("transient")
		}
		return s + "_ok", nil
	}
	out, err := workflow.Retry(5, flaky)(context.Background(), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "x_ok" {
		t.Errorf("unexpected output: %q", out)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_exhausted(t *testing.T) {
	_, err := workflow.Retry(2, fail)(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}

func TestMap(t *testing.T) {
	inputs := []string{"a", "b", "c"}
	outs, err := workflow.Map(context.Background(), inputs, upper)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outs) != 3 || outs[0] != "A" || outs[1] != "B" || outs[2] != "C" {
		t.Errorf("unexpected outputs: %v", outs)
	}
}
