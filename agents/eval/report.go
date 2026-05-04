package eval

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

// Stats is the aggregate outcome of a Suite.Run.
type Stats struct {
	Total          int
	Passed         int
	Failed         int
	Errored        int
	PassAt1        float64       // Passed / Total
	MeanTokens     float64       // average across every task (passed or not)
	MeanTurns      float64       // average across every task
	MeanLatency    time.Duration // average across every task
	P50PassLatency time.Duration // median wall-clock among passing tasks
	P95PassLatency time.Duration // 95th percentile among passing tasks
}

// Report is the result of Suite.Run, per-task TaskResults plus aggregate Stats.
type Report struct {
	Results []TaskResult
	Stats   Stats
}

// Print writes a human-readable summary to w. The format is stable but
// not machine-parseable; callers wanting structured output should marshal
// the Report directly.
func (r *Report) Print(w io.Writer) {
	fmt.Fprintln(w, "=== Eval Report ===")
	fmt.Fprintf(w, "Total: %d   Passed: %d   Failed: %d   Errored: %d   pass@1: %.1f%%\n",
		r.Stats.Total, r.Stats.Passed, r.Stats.Failed, r.Stats.Errored, r.Stats.PassAt1*100)
	fmt.Fprintf(w, "Mean tokens: %.0f   Mean turns: %.1f   Mean latency: %s   p50: %s   p95: %s\n\n",
		r.Stats.MeanTokens, r.Stats.MeanTurns,
		round(r.Stats.MeanLatency), round(r.Stats.P50PassLatency), round(r.Stats.P95PassLatency))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tTOKENS\tTURNS\tLATENCY\tREASON")
	for _, t := range r.Results {
		status := "PASS"
		switch {
		case t.RunErr != nil:
			status = "RUN-ERR"
		case t.ScoreErr != nil:
			status = "JUDGE-ERR"
		case !t.Score.Pass:
			status = "FAIL"
		}
		var tokens, turns string
		if t.Result != nil {
			tokens = fmt.Sprintf("%d", t.Result.Usage.TotalTokens)
			turns = fmt.Sprintf("%d", len(t.Result.Trajectory.Turns))
		}
		reason := t.Score.Reason
		if t.RunErr != nil {
			reason = "run: " + t.RunErr.Error()
		} else if t.ScoreErr != nil {
			reason = "score: " + t.ScoreErr.Error()
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			truncate(t.Task.ID, 30), status, tokens, turns, round(t.Duration), truncate(reason, 60))
	}
	tw.Flush()
}

func round(d time.Duration) time.Duration { return d.Round(time.Millisecond) }

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// WriteCSV emits a one-row-per-task CSV summary suitable for archiving
// across runs. Column order is stable; callers can diff CSVs across
// iterations to track regressions.
func (r *Report) WriteCSV(w io.Writer) error {
	cols := []string{"id", "tags", "status", "tokens", "turns", "duration_ms", "reason"}
	if _, err := fmt.Fprintln(w, strings.Join(cols, ",")); err != nil {
		return err
	}
	for _, t := range r.Results {
		status := "pass"
		switch {
		case t.RunErr != nil:
			status = "run_error"
		case t.ScoreErr != nil:
			status = "score_error"
		case !t.Score.Pass:
			status = "fail"
		}
		var tokens, turns int
		if t.Result != nil {
			tokens = t.Result.Usage.TotalTokens
			turns = len(t.Result.Trajectory.Turns)
		}
		reason := t.Score.Reason
		if t.RunErr != nil {
			reason = "run: " + t.RunErr.Error()
		} else if t.ScoreErr != nil {
			reason = "score: " + t.ScoreErr.Error()
		}
		row := []string{
			csvField(t.Task.ID),
			csvField(strings.Join(t.Task.Tags, ";")),
			status,
			fmt.Sprintf("%d", tokens),
			fmt.Sprintf("%d", turns),
			fmt.Sprintf("%d", t.Duration.Milliseconds()),
			csvField(reason),
		}
		if _, err := fmt.Fprintln(w, strings.Join(row, ",")); err != nil {
			return err
		}
	}
	return nil
}

// csvField escapes a string for safe CSV emission. Quotes cells that
// contain commas, quotes, or newlines; doubles internal quotes.
func csvField(s string) string {
	if !strings.ContainsAny(s, `,"`+"\n") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
