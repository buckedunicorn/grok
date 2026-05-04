package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/buckedunicorn/grok/agents"
)

// Contains passes when the run output contains Substr. CaseInsensitive
// folds both sides to lower case before comparison.
type Contains struct {
	Substr          string
	CaseInsensitive bool
}

// Score satisfies Scorer.
func (c Contains) Score(_ context.Context, result *agents.RunResult) (Score, error) {
	got := result.Output
	want := c.Substr
	if c.CaseInsensitive {
		got = strings.ToLower(got)
		want = strings.ToLower(want)
	}
	if strings.Contains(got, want) {
		return Score{Pass: true, Reason: fmt.Sprintf("output contains %q", c.Substr)}, nil
	}
	return Score{Pass: false, Reason: fmt.Sprintf("output does not contain %q", c.Substr)}, nil
}

// Equal passes when the trimmed run output exactly equals Want.
// CaseInsensitive folds both sides to lower case before comparison.
type Equal struct {
	Want            string
	CaseInsensitive bool
}

// Score satisfies Scorer.
func (e Equal) Score(_ context.Context, result *agents.RunResult) (Score, error) {
	got := strings.TrimSpace(result.Output)
	want := strings.TrimSpace(e.Want)
	if e.CaseInsensitive {
		got = strings.ToLower(got)
		want = strings.ToLower(want)
	}
	if got == want {
		return Score{Pass: true, Reason: "exact match"}, nil
	}
	return Score{Pass: false, Reason: fmt.Sprintf("got %q, want %q", got, want)}, nil
}

// Regex passes when the run output matches Pattern. The pattern is
// compiled once on the first Score call and cached; subsequent Score
// calls reuse the *regexp.Regexp.
//
// Score has a pointer receiver so the cache survives across calls;
// pass &eval.Regex{Pattern: "..."} when constructing a Suite.
type Regex struct {
	Pattern string

	once sync.Once
	re   *regexp.Regexp
	err  error
}

// Score satisfies Scorer.
func (r *Regex) Score(_ context.Context, result *agents.RunResult) (Score, error) {
	r.once.Do(func() {
		r.re, r.err = regexp.Compile(r.Pattern)
	})
	if r.err != nil {
		return Score{}, fmt.Errorf("eval: compile regex %q: %w", r.Pattern, r.err)
	}
	if r.re.MatchString(result.Output) {
		return Score{Pass: true, Reason: fmt.Sprintf("matched %q", r.Pattern)}, nil
	}
	return Score{Pass: false, Reason: fmt.Sprintf("no match for %q", r.Pattern)}, nil
}

// JSONField passes when the run output parses as JSON and the value at
// FieldPath equals Want. FieldPath is a dot-separated path (e.g. "user.name")
// applied against the parsed JSON map; numeric indices in arrays are
// supported (e.g. "items.0.id").
type JSONField struct {
	FieldPath string
	Want      any
}

// Score satisfies Scorer.
func (j JSONField) Score(_ context.Context, result *agents.RunResult) (Score, error) {
	var v any
	if err := json.Unmarshal([]byte(result.Output), &v); err != nil {
		return Score{Pass: false, Reason: fmt.Sprintf("output is not valid JSON: %v", err)}, nil
	}
	got, ok := lookupPath(v, j.FieldPath)
	if !ok {
		return Score{Pass: false, Reason: fmt.Sprintf("path %q not found", j.FieldPath)}, nil
	}
	if !looseEqual(got, j.Want) {
		return Score{Pass: false, Reason: fmt.Sprintf("at %q got %v (%T), want %v (%T)", j.FieldPath, got, got, j.Want, j.Want)}, nil
	}
	return Score{Pass: true, Reason: fmt.Sprintf("at %q == %v", j.FieldPath, j.Want)}, nil
}

// AllOf passes only when every wrapped Scorer passes. Stops at the first
// failure (including infrastructure errors) for efficiency.
type AllOf []Scorer

// Score satisfies Scorer.
func (a AllOf) Score(ctx context.Context, result *agents.RunResult) (Score, error) {
	for i, s := range a {
		sc, err := s.Score(ctx, result)
		if err != nil {
			return Score{}, fmt.Errorf("AllOf[%d]: %w", i, err)
		}
		if !sc.Pass {
			return Score{Pass: false, Reason: fmt.Sprintf("AllOf[%d] failed: %s", i, sc.Reason)}, nil
		}
	}
	return Score{Pass: true, Reason: fmt.Sprintf("all %d scorers passed", len(a))}, nil
}

// AnyOf passes when at least one wrapped Scorer passes. Returns the first
// passing reason; if none pass, returns the last failure reason.
type AnyOf []Scorer

// Score satisfies Scorer.
func (a AnyOf) Score(ctx context.Context, result *agents.RunResult) (Score, error) {
	var lastReason string
	for i, s := range a {
		sc, err := s.Score(ctx, result)
		if err != nil {
			return Score{}, fmt.Errorf("AnyOf[%d]: %w", i, err)
		}
		if sc.Pass {
			return Score{Pass: true, Reason: fmt.Sprintf("AnyOf[%d] passed: %s", i, sc.Reason)}, nil
		}
		lastReason = sc.Reason
	}
	if len(a) == 0 {
		return Score{Pass: false, Reason: "AnyOf: no scorers"}, nil
	}
	return Score{Pass: false, Reason: fmt.Sprintf("none of %d scorers passed; last: %s", len(a), lastReason)}, nil
}

// Not inverts the wrapped Scorer's Pass field.
type Not struct{ Scorer Scorer }

// Score satisfies Scorer.
func (n Not) Score(ctx context.Context, result *agents.RunResult) (Score, error) {
	sc, err := n.Scorer.Score(ctx, result)
	if err != nil {
		return Score{}, fmt.Errorf("Not: %w", err)
	}
	return Score{Pass: !sc.Pass, Reason: "Not: " + sc.Reason}, nil
}

// --- internal helpers ---

// lookupPath walks v using a dot-separated path. Returns the final value
// and true if found; the zero value and false if any segment is missing.
// Map keys are matched as strings; numeric segments index into JSON arrays.
func lookupPath(v any, path string) (any, bool) {
	if path == "" {
		return v, true
	}
	for seg := range strings.SplitSeq(path, ".") {
		switch cur := v.(type) {
		case map[string]any:
			next, ok := cur[seg]
			if !ok {
				return nil, false
			}
			v = next
		case []any:
			idx, err := indexOf(seg)
			if err != nil || idx < 0 || idx >= len(cur) {
				return nil, false
			}
			v = cur[idx]
		default:
			return nil, false
		}
	}
	return v, true
}

// indexOf parses a non-negative integer from s. Returns -1 on failure
// (no panic for non-numeric segments).
func indexOf(s string) (int, error) {
	n := 0
	if s == "" {
		return -1, fmt.Errorf("empty index")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1, fmt.Errorf("not a digit: %q", c)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// looseEqual handles the common JSON-decode types. JSON numbers decode
// as float64 even when the Want side is an int, so we compare numerics
// loosely. Strings, bools, and nils compare strictly. The fallback uses
// reflect.DeepEqual rather than fmt.Sprintf("%v", ...) twice, which
// allocated two strings per comparison.
func looseEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	switch ax := a.(type) {
	case float64:
		switch bx := b.(type) {
		case float64:
			return ax == bx
		case int:
			return ax == float64(bx)
		case int64:
			return ax == float64(bx)
		}
	case string:
		bx, ok := b.(string)
		return ok && ax == bx
	case bool:
		bx, ok := b.(bool)
		return ok && ax == bx
	}
	return reflect.DeepEqual(a, b)
}
