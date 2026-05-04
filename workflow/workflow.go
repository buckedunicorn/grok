// Package workflow provides composable building blocks for multi-step AI pipelines.
//
// A Step is any function that transforms a string input into a string output.
// Steps compose via Chain (sequential), Parallel (concurrent), and Conditional.
//
//	result, err := workflow.Chain(ctx, userInput,
//	    llmSummarize,
//	    llmTranslate,
//	    workflow.Transform(strings.ToUpper),
//	)
package workflow

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

// Step transforms a string input and returns a string output.
// It is the basic unit of a workflow pipeline.
type Step func(ctx context.Context, input string) (string, error)

// Chain runs steps sequentially, passing each output as the next step's input.
// Returns the output of the final step, or the first error encountered.
func Chain(ctx context.Context, input string, steps ...Step) (string, error) {
	cur := input
	for i, step := range steps {
		out, err := step(ctx, cur)
		if err != nil {
			return "", fmt.Errorf("workflow: step %d: %w", i, err)
		}
		cur = out
	}
	return cur, nil
}

// Parallel runs all steps concurrently with the same input.
// Returns all outputs in the same order as steps, or the first error that
// cancels all remaining steps.
func Parallel(ctx context.Context, input string, steps ...Step) ([]string, error) {
	outputs := make([]string, len(steps))
	g, gctx := errgroup.WithContext(ctx)
	for i, step := range steps {
		i, step := i, step
		g.Go(func() error {
			out, err := step(gctx, input)
			if err != nil {
				return fmt.Errorf("workflow: step %d: %w", i, err)
			}
			outputs[i] = out
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return outputs, nil
}

// Conditional returns a Step that calls thenStep when condition(input) is true,
// or elseStep otherwise. Pass nil for elseStep to pass input through unchanged.
func Conditional(condition func(string) bool, thenStep, elseStep Step) Step {
	return func(ctx context.Context, input string) (string, error) {
		if condition(input) {
			return thenStep(ctx, input)
		}
		if elseStep != nil {
			return elseStep(ctx, input)
		}
		return input, nil
	}
}

// Transform wraps a pure string transformation as a Step.
func Transform(fn func(string) string) Step {
	return func(_ context.Context, input string) (string, error) {
		return fn(input), nil
	}
}

// Retry wraps a Step, re-running it up to maxAttempts times on error.
func Retry(maxAttempts int, step Step) Step {
	return func(ctx context.Context, input string) (string, error) {
		var lastErr error
		for i := range maxAttempts {
			out, err := step(ctx, input)
			if err == nil {
				return out, nil
			}
			lastErr = err
			_ = i
		}
		return "", fmt.Errorf("workflow: all %d attempts failed: %w", maxAttempts, lastErr)
	}
}

// Map applies step to each element of inputs concurrently and returns results
// in the same order. All elements share a single errgroup; first error cancels all.
func Map(ctx context.Context, inputs []string, step Step) ([]string, error) {
	steps := make([]Step, len(inputs))
	for i, inp := range inputs {
		inp := inp
		steps[i] = func(ctx context.Context, _ string) (string, error) {
			return step(ctx, inp)
		}
	}
	return Parallel(ctx, "", steps...)
}
