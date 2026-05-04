package agents_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/buckedunicorn/grok/agents"
)

// recordingCheckpointer captures every SaveCheckpoint call for assertions.
type recordingCheckpointer struct {
	saved []agents.CheckpointState
	err   error
}

func (r *recordingCheckpointer) SaveCheckpoint(_ context.Context, state agents.CheckpointState) error {
	if r.err != nil {
		return r.err
	}
	r.saved = append(r.saved, state)
	return nil
}

func TestRunner_Checkpointer_savesAfterEachToolTurn(t *testing.T) {
	calls := 0
	srv, c := newChatClient(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		switch calls {
		case 1:
			json.NewEncoder(w).Encode(toolCompletion("tc1", "echo", `{"v":1}`))
		case 2:
			json.NewEncoder(w).Encode(toolCompletion("tc2", "echo", `{"v":2}`))
		case 3:
			json.NewEncoder(w).Encode(textCompletion("done"))
		}
	})
	defer srv.Close()

	rec := &recordingCheckpointer{}
	tool := agents.Tool{
		Name:    "echo",
		Handler: func(_ context.Context, args string) (string, error) { return args, nil },
	}
	r := &agents.Runner{Client: c, Checkpointer: rec}

	res, err := r.Run(context.Background(),
		&agents.Agent{Name: "X", Model: "m", Tools: []agents.Tool{tool}},
		agents.RunOptions{Input: "go"},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Two tool-call turns + one terminal turn = 2 saves (terminal turn doesn't save).
	if len(rec.saved) != 2 {
		t.Errorf("expected 2 checkpoints, got %d", len(rec.saved))
	}
	for i, cp := range rec.saved {
		if cp.RunID != res.Trajectory.RunID {
			t.Errorf("checkpoint[%d] RunID = %q, want %q", i, cp.RunID, res.Trajectory.RunID)
		}
		if cp.NextTurn != i+1 {
			t.Errorf("checkpoint[%d] NextTurn = %d, want %d", i, cp.NextTurn, i+1)
		}
		if cp.AgentName != "X" {
			t.Errorf("checkpoint[%d] AgentName = %q", i, cp.AgentName)
		}
	}
}

func TestRunner_Checkpointer_errorAbortsRun(t *testing.T) {
	calls := 0
	srv, c := newChatClient(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		json.NewEncoder(w).Encode(toolCompletion("tc1", "echo", `{}`))
	})
	defer srv.Close()

	rec := &recordingCheckpointer{err: errors.New("disk full")}
	tool := agents.Tool{
		Name:    "echo",
		Handler: func(_ context.Context, _ string) (string, error) { return "ok", nil },
	}
	r := &agents.Runner{Client: c, Checkpointer: rec}

	_, err := r.Run(context.Background(),
		&agents.Agent{Name: "X", Model: "m", Tools: []agents.Tool{tool}},
		agents.RunOptions{Input: "go"},
	)
	if err == nil {
		t.Fatal("expected error from failed checkpoint")
	}
	// Make sure we didn't continue past the failure.
	if calls != 1 {
		t.Errorf("expected 1 chat call before checkpoint failure, got %d", calls)
	}
}

func TestRunner_Resume_continuesFromCheckpoint(t *testing.T) {
	// Stage 1: run for two tool turns then "crash" (no terminal completion).
	stage := 0
	calls := atomic.Int32{}
	var stage1ChkRec recordingCheckpointer
	{
		srv, c := newChatClient(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			switch stage*100 + int(calls.Load()) {
			case 1:
				json.NewEncoder(w).Encode(toolCompletion("tc1", "echo", `{"v":1}`))
			case 2:
				json.NewEncoder(w).Encode(toolCompletion("tc2", "echo", `{"v":2}`))
			default:
				t.Errorf("unexpected stage1 call #%d", calls.Load())
				http.Error(w, "no", 500)
			}
		})
		defer srv.Close()

		// Use a Checkpointer that aborts after the second save so the run
		// terminates without producing a final answer, like a crash.
		callCount := 0
		stage1Cp := agents.CheckpointerFunc(func(_ context.Context, state agents.CheckpointState) error {
			stage1ChkRec.saved = append(stage1ChkRec.saved, state)
			callCount++
			if callCount >= 2 {
				return errors.New("simulated crash")
			}
			return nil
		})
		tool := agents.Tool{
			Name:    "echo",
			Handler: func(_ context.Context, args string) (string, error) { return args, nil },
		}
		r := &agents.Runner{Client: c, Checkpointer: stage1Cp}
		_, err := r.Run(context.Background(),
			&agents.Agent{Name: "X", Model: "m", Tools: []agents.Tool{tool}},
			agents.RunOptions{Input: "go"},
		)
		if err == nil {
			t.Fatal("expected stage1 to error from simulated crash")
		}
	}

	if len(stage1ChkRec.saved) != 2 {
		t.Fatalf("stage1 expected 2 checkpoints, got %d", len(stage1ChkRec.saved))
	}
	last := stage1ChkRec.saved[len(stage1ChkRec.saved)-1]

	// Stage 2: resume from the last checkpoint with a fresh client that
	// only handles the resumed terminal turn.
	stage = 1
	calls.Store(0)
	srv2, c2 := newChatClient(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		// On resume we expect exactly one terminal completion.
		json.NewEncoder(w).Encode(textCompletion("resumed answer"))
	})
	defer srv2.Close()

	tool := agents.Tool{
		Name:    "echo",
		Handler: func(_ context.Context, args string) (string, error) { return args, nil },
	}
	r2 := &agents.Runner{Client: c2}
	res, err := r2.Run(context.Background(),
		&agents.Agent{Name: "X", Model: "m", Tools: []agents.Tool{tool}},
		agents.RunOptions{Resume: &last},
	)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "resumed answer" {
		t.Errorf("Output = %q", res.Output)
	}
	// Trajectory should preserve the original RunID and prepend stage1's turns.
	if res.Trajectory.RunID != last.RunID {
		t.Errorf("RunID changed: %q -> %q", last.RunID, res.Trajectory.RunID)
	}
	if got := len(res.Trajectory.Turns); got != 3 { // 2 from stage1 + 1 terminal
		t.Errorf("expected 3 turns total, got %d", got)
	}
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 chat call on resume, got %d", calls.Load())
	}
}

func TestRunner_Resume_rejectsSessionConflict(t *testing.T) {
	srv, c := newChatClient(func(_ http.ResponseWriter, _ *http.Request) {})
	defer srv.Close()
	r := &agents.Runner{Client: c}
	_, err := r.Run(context.Background(),
		&agents.Agent{Name: "X", Model: "m"},
		agents.RunOptions{
			Resume:  &agents.CheckpointState{RunID: "x"},
			Session: agents.NewInMemorySession("x"),
		},
	)
	if err == nil {
		t.Error("expected error when both Resume and Session are set")
	}
}
