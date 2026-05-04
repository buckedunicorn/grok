// Package eval scores agents.RunResult values produced by agents.Runner.
//
// The package is narrowly scoped: given a Trajectory (or its enclosing
// RunResult), produce a pass/fail Score; given a slice of Tasks and a
// runner, produce aggregate Stats. It does NOT try to be a benchmark
// harness, no test discovery, no result persistence, no leaderboard.
// Compose it with the user's existing test framework or CI pipeline.
//
// Pair each run with a falsifiable Scorer, aggregate outcomes, learn
// from the deltas.
//
// Two invocation patterns:
//
//  1. Score a single result you already have:
//
//     scorer := eval.Contains{Substr: "42"}
//     score, _ := scorer.Score(ctx, result)
//     fmt.Println(score.Pass, score.Reason)
//
//  2. Run a suite of tasks in parallel and aggregate:
//
//     suite := &eval.Suite{
//     Tasks: []eval.Task{
//     {ID: "t1", Input: "what is 2+2?", Expect: eval.Contains{Substr: "4"}},
//     {ID: "t2", Input: "capital of france?", Expect: eval.Contains{Substr: "Paris"}},
//     },
//     Concurrency: 4,
//     }
//     report, _ := suite.Run(ctx, runFn)
//     report.Print(os.Stdout)
package eval

import (
	"context"
	"time"

	"github.com/buckedunicorn/grok/agents"
)

// Scorer judges whether a run satisfied its task. Implementations should
// be pure, same RunResult should always produce the same Score modulo
// genuinely non-deterministic scorers like LLMJudge. Errors are reserved
// for infrastructure failure (e.g. judge model call failed); a clean
// "this run did not pass" should be a Score with Pass: false, not an
// error.
type Scorer interface {
	Score(ctx context.Context, result *agents.RunResult) (Score, error)
}

// ScorerFunc adapts a plain function as a Scorer.
type ScorerFunc func(ctx context.Context, result *agents.RunResult) (Score, error)

// Score satisfies Scorer for ScorerFunc.
func (f ScorerFunc) Score(ctx context.Context, result *agents.RunResult) (Score, error) {
	return f(ctx, result)
}

// Score is the verdict on a single run.
type Score struct {
	Pass   bool
	Reason string         // short human-readable explanation, e.g. "contains 'Paris'"
	Notes  map[string]any // optional structured metadata: confidence, judge tokens, etc.
}

// Task pairs an input prompt with the Scorer that judges the result.
type Task struct {
	ID     string
	Input  string
	Expect Scorer
	// Tags are free-form labels for filtering / grouping in reports
	// (e.g. "math", "factual", "regression-2026-04").
	Tags []string
}

// RunFunc is the signature suite runners use to execute a single task.
// Typical wiring delegates to agents.Runner.Run or harness.Run:
//
//	runFn := func(ctx context.Context, task eval.Task) (*agents.RunResult, error) {
//	    return runner.Run(ctx, agent, agents.RunOptions{Input: task.Input})
//	}
type RunFunc func(ctx context.Context, task Task) (*agents.RunResult, error)

// TaskResult is one task's outcome plus its score and timing.
type TaskResult struct {
	Task     Task
	Result   *agents.RunResult // nil if RunErr is set
	Score    Score             // zero value if either RunErr or ScoreErr is set
	RunErr   error
	ScoreErr error
	Duration time.Duration
}

// OK reports whether both the run and the scoring succeeded AND the score
// passed. Convenient for filtering report rows.
func (t *TaskResult) OK() bool {
	return t.RunErr == nil && t.ScoreErr == nil && t.Score.Pass
}
