package eval

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"
)

// Suite is a collection of Tasks evaluated against a single RunFunc.
// Concurrency caps how many tasks run at once; 0 or negative means
// sequential.
type Suite struct {
	Tasks       []Task
	Concurrency int
}

// Run executes every Task via runFn, scores each result with the task's
// Expect Scorer, and returns a Report. ctx cancellation aborts pending
// tasks; tasks already in-flight respect ctx through runFn.
//
// Run never returns a hard error for a single task failing, those are
// captured in Report.Results. The returned error is reserved for
// configuration problems detected up front (e.g. nil runFn).
func (s *Suite) Run(ctx context.Context, runFn RunFunc) (*Report, error) {
	if runFn == nil {
		return nil, errors.New("eval: Suite.Run runFn is nil")
	}

	results := make([]TaskResult, len(s.Tasks))
	concurrency := s.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, task := range s.Tasks {
		select {
		case <-ctx.Done():
			// Mark remaining tasks as cancelled.
			for j := i; j < len(s.Tasks); j++ {
				results[j] = TaskResult{Task: s.Tasks[j], RunErr: ctx.Err()}
			}
			wg.Wait()
			return buildReport(results), nil
		default:
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(i int, task Task) {
			defer wg.Done()
			defer func() { <-sem }()

			start := time.Now()
			result, runErr := runFn(ctx, task)
			tr := TaskResult{
				Task:     task,
				Result:   result,
				RunErr:   runErr,
				Duration: time.Since(start),
			}
			if runErr == nil && task.Expect != nil {
				score, scoreErr := task.Expect.Score(ctx, result)
				tr.Score = score
				tr.ScoreErr = scoreErr
			}
			results[i] = tr
		}(i, task)
	}

	wg.Wait()
	return buildReport(results), nil
}

// buildReport assembles aggregate stats from per-task results.
func buildReport(results []TaskResult) *Report {
	stats := Stats{Total: len(results)}
	var totalTokens, totalTurns int
	var totalLatency time.Duration
	var passLatencies []time.Duration

	for _, r := range results {
		switch {
		case r.RunErr != nil || r.ScoreErr != nil:
			stats.Errored++
		case r.Score.Pass:
			stats.Passed++
		default:
			stats.Failed++
		}
		if r.Result != nil {
			totalTokens += r.Result.Usage.TotalTokens
			totalTurns += len(r.Result.Trajectory.Turns)
		}
		totalLatency += r.Duration
		if r.OK() {
			passLatencies = append(passLatencies, r.Duration)
		}
	}

	if stats.Total > 0 {
		stats.PassAt1 = float64(stats.Passed) / float64(stats.Total)
		stats.MeanTokens = float64(totalTokens) / float64(stats.Total)
		stats.MeanTurns = float64(totalTurns) / float64(stats.Total)
		stats.MeanLatency = totalLatency / time.Duration(stats.Total)
	}
	if len(passLatencies) > 0 {
		slices.Sort(passLatencies)
		stats.P50PassLatency = passLatencies[len(passLatencies)/2]
		idx := (len(passLatencies) * 95) / 100
		if idx >= len(passLatencies) {
			idx = len(passLatencies) - 1
		}
		stats.P95PassLatency = passLatencies[idx]
	}

	return &Report{Results: results, Stats: stats}
}
