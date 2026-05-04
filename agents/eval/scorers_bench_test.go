package eval_test

import (
	"context"
	"testing"

	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/agents/eval"
)

// BenchmarkRegex_Score validates after the first
// Score() call the regex is cached, so subsequent calls do no
// regexp.Compile and allocate only the Reason string.
func BenchmarkRegex_Score(b *testing.B) {
	s := &eval.Regex{Pattern: `\b(error|fail|panic)\b`}
	res := &agents.RunResult{Output: "everything ran successfully without any error or panic events."}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = s.Score(ctx, res)
	}
}

// BenchmarkContains_Score is a baseline reference; Contains never
// compiled anything per-call, so it should look similar to the
// post-fix Regex.
func BenchmarkContains_Score(b *testing.B) {
	s := eval.Contains{Substr: "panic"}
	res := &agents.RunResult{Output: "everything ran successfully without any panic events."}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = s.Score(ctx, res)
	}
}
