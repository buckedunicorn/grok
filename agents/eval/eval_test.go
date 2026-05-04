package eval_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/agents/eval"
	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/internal/transport"
)

func mkResult(output string) *agents.RunResult {
	return &agents.RunResult{
		Output: output,
		Usage:  chat.Usage{TotalTokens: 100},
		Trajectory: agents.Trajectory{
			Turns: []agents.Turn{{Index: 0}},
		},
	}
}

// --- scorer tests ---

func TestContains(t *testing.T) {
	s := eval.Contains{Substr: "Paris"}
	got, err := s.Score(context.Background(), mkResult("The capital is Paris."))
	if err != nil || !got.Pass {
		t.Errorf("expected pass, got %+v err=%v", got, err)
	}
	got, _ = s.Score(context.Background(), mkResult("nope"))
	if got.Pass {
		t.Error("expected fail")
	}
}

func TestContains_caseInsensitive(t *testing.T) {
	s := eval.Contains{Substr: "PARIS", CaseInsensitive: true}
	got, _ := s.Score(context.Background(), mkResult("The capital is paris."))
	if !got.Pass {
		t.Error("expected pass with CaseInsensitive")
	}
}

func TestEqual_trimsWhitespace(t *testing.T) {
	s := eval.Equal{Want: "42"}
	got, _ := s.Score(context.Background(), mkResult("  42\n"))
	if !got.Pass {
		t.Errorf("expected pass after trim, got %+v", got)
	}
}

func TestRegex(t *testing.T) {
	s := eval.Regex{Pattern: `\b\d{3}-\d{4}\b`}
	got, _ := s.Score(context.Background(), mkResult("call 555-1234 for help"))
	if !got.Pass {
		t.Error("expected pass")
	}
	got, _ = s.Score(context.Background(), mkResult("no phone here"))
	if got.Pass {
		t.Error("expected fail")
	}
}

func TestRegex_invalidPattern(t *testing.T) {
	s := eval.Regex{Pattern: `[unclosed`}
	_, err := s.Score(context.Background(), mkResult("anything"))
	if err == nil {
		t.Error("expected compile error")
	}
}

func TestJSONField_topLevelString(t *testing.T) {
	s := eval.JSONField{FieldPath: "name", Want: "Paris"}
	got, _ := s.Score(context.Background(), mkResult(`{"name":"Paris","pop":2.1}`))
	if !got.Pass {
		t.Errorf("expected pass, got %+v", got)
	}
}

func TestJSONField_nestedNumeric(t *testing.T) {
	// JSON decodes numbers as float64; JSONField should compare loosely
	// against an int Want.
	s := eval.JSONField{FieldPath: "data.count", Want: 5}
	got, _ := s.Score(context.Background(), mkResult(`{"data":{"count":5}}`))
	if !got.Pass {
		t.Errorf("expected pass with loose numeric, got %+v", got)
	}
}

func TestJSONField_arrayIndex(t *testing.T) {
	s := eval.JSONField{FieldPath: "items.1.id", Want: "second"}
	got, _ := s.Score(context.Background(), mkResult(`{"items":[{"id":"first"},{"id":"second"}]}`))
	if !got.Pass {
		t.Errorf("expected pass, got %+v", got)
	}
}

func TestJSONField_pathMissing(t *testing.T) {
	s := eval.JSONField{FieldPath: "nope", Want: "x"}
	got, _ := s.Score(context.Background(), mkResult(`{"name":"y"}`))
	if got.Pass {
		t.Errorf("expected fail for missing path, got %+v", got)
	}
}

func TestJSONField_invalidJSON(t *testing.T) {
	s := eval.JSONField{FieldPath: "x", Want: 1}
	got, _ := s.Score(context.Background(), mkResult("not json at all"))
	if got.Pass {
		t.Error("expected fail for non-JSON output")
	}
}

func TestAllOf(t *testing.T) {
	s := eval.AllOf{
		eval.Contains{Substr: "foo"},
		eval.Contains{Substr: "bar"},
	}
	got, _ := s.Score(context.Background(), mkResult("foo and bar"))
	if !got.Pass {
		t.Error("expected pass")
	}
	got, _ = s.Score(context.Background(), mkResult("foo only"))
	if got.Pass {
		t.Error("expected fail when one scorer fails")
	}
}

func TestAnyOf(t *testing.T) {
	s := eval.AnyOf{
		eval.Contains{Substr: "absent"},
		eval.Contains{Substr: "present"},
	}
	got, _ := s.Score(context.Background(), mkResult("the keyword present is here"))
	if !got.Pass {
		t.Error("expected pass when one scorer passes")
	}
	got, _ = s.Score(context.Background(), mkResult("nothing"))
	if got.Pass {
		t.Error("expected fail when none pass")
	}
}

func TestNot(t *testing.T) {
	inner := eval.Contains{Substr: "forbidden"}
	s := eval.Not{Scorer: inner}
	got, _ := s.Score(context.Background(), mkResult("clean output"))
	if !got.Pass {
		t.Error("Not should invert: inner=fail -> Not=pass")
	}
	got, _ = s.Score(context.Background(), mkResult("forbidden word"))
	if got.Pass {
		t.Error("Not should invert: inner=pass -> Not=fail")
	}
}

// --- suite tests ---

func TestSuite_runsAllAndAggregates(t *testing.T) {
	tasks := []eval.Task{
		{ID: "a", Input: "1", Expect: eval.Equal{Want: "one"}},
		{ID: "b", Input: "2", Expect: eval.Equal{Want: "two"}},
		{ID: "c", Input: "3", Expect: eval.Equal{Want: "three"}},
	}
	runFn := func(_ context.Context, task eval.Task) (*agents.RunResult, error) {
		switch task.ID {
		case "a":
			return mkResult("one"), nil
		case "b":
			return mkResult("WRONG"), nil
		case "c":
			return nil, errors.New("api down")
		}
		return nil, nil
	}

	suite := &eval.Suite{Tasks: tasks, Concurrency: 2}
	report, err := suite.Run(context.Background(), runFn)
	if err != nil {
		t.Fatal(err)
	}
	if report.Stats.Total != 3 || report.Stats.Passed != 1 || report.Stats.Failed != 1 || report.Stats.Errored != 1 {
		t.Errorf("stats = %+v", report.Stats)
	}
	if report.Stats.PassAt1 != 1.0/3.0 {
		t.Errorf("PassAt1 = %v", report.Stats.PassAt1)
	}
}

func TestSuite_concurrentExecution(t *testing.T) {
	const n = 8
	tasks := make([]eval.Task, n)
	for i := range n {
		tasks[i] = eval.Task{
			ID:     string(rune('a' + i)),
			Expect: eval.Equal{Want: "ok"},
		}
	}
	var inflight, peak atomic.Int32
	runFn := func(_ context.Context, _ eval.Task) (*agents.RunResult, error) {
		cur := inflight.Add(1)
		defer inflight.Add(-1)
		// Track peak concurrency.
		for {
			old := peak.Load()
			if cur <= old || peak.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		return mkResult("ok"), nil
	}

	suite := &eval.Suite{Tasks: tasks, Concurrency: 4}
	if _, err := suite.Run(context.Background(), runFn); err != nil {
		t.Fatal(err)
	}
	if got := peak.Load(); got > 4 {
		t.Errorf("peak concurrency = %d, want <= 4", got)
	}
}

func TestSuite_ctxCancellation(t *testing.T) {
	tasks := []eval.Task{
		{ID: "slow", Expect: eval.Equal{Want: "x"}},
	}
	runFn := func(ctx context.Context, _ eval.Task) (*agents.RunResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	suite := &eval.Suite{Tasks: tasks}
	report, err := suite.Run(ctx, runFn)
	if err != nil {
		t.Fatal(err)
	}
	if report.Stats.Errored == 0 {
		t.Errorf("expected errored task on ctx cancellation, got %+v", report.Stats)
	}
}

func TestSuite_nilRunFn(t *testing.T) {
	suite := &eval.Suite{Tasks: []eval.Task{{ID: "x"}}}
	_, err := suite.Run(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil runFn")
	}
}

// --- report tests ---

func TestReport_PrintFormatStable(t *testing.T) {
	report := &eval.Report{
		Results: []eval.TaskResult{
			{Task: eval.Task{ID: "t1"}, Result: mkResult("ok"), Score: eval.Score{Pass: true, Reason: "ok"}, Duration: time.Millisecond * 50},
			{Task: eval.Task{ID: "t2"}, Result: mkResult("nope"), Score: eval.Score{Pass: false, Reason: "miss"}, Duration: time.Millisecond * 30},
		},
		Stats: eval.Stats{Total: 2, Passed: 1, Failed: 1, PassAt1: 0.5},
	}
	var buf bytes.Buffer
	report.Print(&buf)
	out := buf.String()
	for _, want := range []string{"pass@1: 50.0%", "PASS", "FAIL", "t1", "t2"} {
		if !strings.Contains(out, want) {
			t.Errorf("Print output missing %q\n%s", want, out)
		}
	}
}

func TestReport_WriteCSV(t *testing.T) {
	report := &eval.Report{
		Results: []eval.TaskResult{
			{Task: eval.Task{ID: "t1", Tags: []string{"math", "easy"}}, Result: mkResult("ok"), Score: eval.Score{Pass: true, Reason: "ok"}, Duration: time.Millisecond * 50},
		},
	}
	var buf bytes.Buffer
	if err := report.WriteCSV(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "t1") || !strings.Contains(out, "math;easy") {
		t.Errorf("CSV missing expected fields:\n%s", out)
	}
	if !strings.Contains(out, "id,tags,status") {
		t.Errorf("CSV header missing:\n%s", out)
	}
}

func TestReport_WriteCSV_quotesSpecialChars(t *testing.T) {
	report := &eval.Report{
		Results: []eval.TaskResult{
			{Task: eval.Task{ID: "weird,id"}, Result: mkResult("x"), Score: eval.Score{Pass: false, Reason: `with "quotes" and, commas`}, Duration: time.Millisecond},
		},
	}
	var buf bytes.Buffer
	_ = report.WriteCSV(&buf)
	out := buf.String()
	if !strings.Contains(out, `"weird,id"`) {
		t.Errorf("ID with comma should be quoted:\n%s", out)
	}
	if !strings.Contains(out, `""quotes""`) {
		t.Errorf("internal quotes should be doubled:\n%s", out)
	}
}

// --- judge tests ---

func TestLLMJudge_passesVerdict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{
				FinishReason: "stop",
				Message:      chat.Message{Role: "assistant", Content: `{"pass":true,"reason":"answer matches expected","confidence":0.95}`},
			}},
			Usage: chat.Usage{TotalTokens: 50},
		})
	}))
	defer srv.Close()

	tr := transport.NewInsecure("test-key", srv.URL, srv.Client())
	c := chat.NewClient(tr)

	judge := eval.LLMJudge{
		Client: c,
		Model:  "test-judge-model",
		Rubric: "Pass if the output answers the question correctly.",
	}
	got, err := judge.Score(context.Background(), mkResult("the answer is 4"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Pass {
		t.Errorf("expected pass, got %+v", got)
	}
	if got.Notes["judge_confidence"] != 0.95 {
		t.Errorf("confidence = %v", got.Notes["judge_confidence"])
	}
}

func TestLLMJudge_invalidVerdictJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{{
				FinishReason: "stop",
				Message:      chat.Message{Role: "assistant", Content: `not json at all`},
			}},
		})
	}))
	defer srv.Close()
	tr := transport.NewInsecure("test-key", srv.URL, srv.Client())
	c := chat.NewClient(tr)

	judge := eval.LLMJudge{Client: c, Model: "m", Rubric: "x"}
	_, err := judge.Score(context.Background(), mkResult("anything"))
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestLLMJudge_requiresConfig(t *testing.T) {
	cases := []eval.LLMJudge{
		{},                                    // all empty
		{Model: "m", Rubric: "r"},             // missing Client
		{Client: &chat.Client{}, Rubric: "r"}, // missing Model
		{Client: &chat.Client{}, Model: "m"},  // missing Rubric
	}
	for i, j := range cases {
		_, err := j.Score(context.Background(), mkResult("x"))
		if err == nil {
			t.Errorf("case %d: expected config error", i)
		}
	}
}

func TestTaskResult_OK(t *testing.T) {
	tr := eval.TaskResult{Score: eval.Score{Pass: true}}
	if !tr.OK() {
		t.Error("OK should be true when Pass with no errors")
	}
	tr.RunErr = errors.New("x")
	if tr.OK() {
		t.Error("OK should be false on RunErr")
	}
}
