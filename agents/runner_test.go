package agents_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/internal/transport"
)

func newChatClient(handler http.HandlerFunc) (*httptest.Server, *chat.Client) {
	srv := httptest.NewServer(handler)
	t := transport.NewInsecure("test-key", srv.URL, srv.Client())
	return srv, chat.NewClient(t)
}

func textCompletion(content string) chat.Completion {
	return chat.Completion{
		Choices: []chat.Choice{{
			FinishReason: "stop",
			Message:      chat.Message{Role: "assistant", Content: content},
		}},
		Usage: chat.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
}

func toolCompletion(toolCallID, name, args string) chat.Completion {
	return chat.Completion{
		Choices: []chat.Choice{{
			FinishReason: "tool_calls",
			Message: chat.Message{
				Role: "assistant",
				ToolCalls: []chat.ToolCall{{
					ID:       toolCallID,
					Type:     "function",
					Function: chat.FunctionCallData{Name: name, Arguments: args},
				}},
			},
		}},
		Usage: chat.Usage{PromptTokens: 20, CompletionTokens: 8, TotalTokens: 28},
	}
}

func TestRunner_noTools_returnsFinalText(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(textCompletion("hello"))
	})
	defer srv.Close()

	r := &agents.Runner{Client: c}
	res, err := r.Run(context.Background(), &agents.Agent{
		Name:  "Helper",
		Model: "m",
	}, agents.RunOptions{Input: "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Output != "hello" {
		t.Errorf("output = %q, want %q", res.Output, "hello")
	}
	if res.LastAgent.Name != "Helper" {
		t.Errorf("LastAgent.Name = %q, want %q", res.LastAgent.Name, "Helper")
	}
	if got := len(res.Trajectory.Turns); got != 1 {
		t.Errorf("expected 1 turn in trajectory, got %d", got)
	}
	if res.Trajectory.RunID == "" {
		t.Error("trajectory missing RunID")
	}
}

func TestRunner_singleToolDispatch(t *testing.T) {
	calls := 0
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			json.NewEncoder(w).Encode(toolCompletion("tc1", "add", `{"a":1,"b":2}`))
		case 2:
			var req chat.CreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			// system + user + assistant(toolcalls) + tool
			if got := len(req.Messages); got != 4 {
				t.Errorf("turn 2: want 4 messages, got %d", got)
			}
			last := req.Messages[len(req.Messages)-1]
			if last.Role != "tool" || last.Content != `3` {
				t.Errorf("unexpected last message: %+v", last)
			}
			json.NewEncoder(w).Encode(textCompletion("the answer is 3"))
		}
	})
	defer srv.Close()

	add := agents.Tool{
		Name: "add",
		Handler: func(_ context.Context, args string) (string, error) {
			var p struct{ A, B int }
			json.Unmarshal([]byte(args), &p)
			return "3", nil // hardcoded for test simplicity
		},
	}

	r := &agents.Runner{Client: c}
	res, err := r.Run(context.Background(), &agents.Agent{
		Name:         "Math",
		Model:        "m",
		Instructions: "You are a math helper.",
		Tools:        []agents.Tool{add},
	}, agents.RunOptions{Input: "what is 1+2?"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Output != "the answer is 3" {
		t.Errorf("output = %q", res.Output)
	}
	if got := len(res.Trajectory.Turns); got != 2 {
		t.Errorf("expected 2 turns, got %d", got)
	}
	if got := len(res.Trajectory.Turns[0].ToolCalls); got != 1 {
		t.Errorf("expected 1 tool call in turn 0, got %d", got)
	}
	tc := res.Trajectory.Turns[0].ToolCalls[0]
	if tc.Name != "add" || tc.Result != "3" {
		t.Errorf("unexpected tool call trace: %+v", tc)
	}
	if tc.ID != "tc1" {
		t.Errorf("tool call ID = %q, want tc1", tc.ID)
	}
	// usage accumulates across turns: 28 + 15 = 43
	if res.Usage.TotalTokens != 43 {
		t.Errorf("total tokens = %d, want 43", res.Usage.TotalTokens)
	}
}

func TestRunner_maxTurnsExceeded(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(toolCompletion("tcN", "loop", `{}`))
	})
	defer srv.Close()

	loop := agents.Tool{
		Name:    "loop",
		Handler: func(_ context.Context, _ string) (string, error) { return `"again"`, nil },
	}
	r := &agents.Runner{Client: c, MaxTurns: 3}
	_, err := r.Run(context.Background(), &agents.Agent{
		Name:  "Looper",
		Model: "m",
		Tools: []agents.Tool{loop},
	}, agents.RunOptions{Input: "go"})
	if !errors.Is(err, agents.ErrMaxTurnsExceeded) {
		t.Fatalf("err = %v, want ErrMaxTurnsExceeded", err)
	}
}

func TestRunner_unknownToolReturnsError(t *testing.T) {
	calls := 0
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			json.NewEncoder(w).Encode(toolCompletion("tc1", "missing_tool", `{}`))
			return
		}
		// turn 2: confirm we see the error result
		var req chat.CreateRequest
		json.NewDecoder(r.Body).Decode(&req)
		last := req.Messages[len(req.Messages)-1]
		if last.Role != "tool" || last.Content == "" {
			t.Errorf("expected tool error result, got %+v", last)
		}
		json.NewEncoder(w).Encode(textCompletion("done"))
	})
	defer srv.Close()

	r := &agents.Runner{Client: c}
	res, err := r.Run(context.Background(), &agents.Agent{Name: "X", Model: "m"}, agents.RunOptions{Input: "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Trajectory.Turns[0].ToolCalls[0].Err == nil {
		t.Error("expected ToolCallTrace.Err to record unknown-tool error")
	}
}

func TestRunner_inputGuardrailTrips(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when input guardrail trips")
	})
	defer srv.Close()

	denyAll := agents.InputGuardrail{
		Name: "deny-all",
		Run:  func(_ context.Context, _ string) error { return errors.New("nope") },
	}
	r := &agents.Runner{Client: c}
	_, err := r.Run(context.Background(), &agents.Agent{
		Name:       "Guarded",
		Model:      "m",
		Guardrails: agents.Guardrails{Input: []agents.InputGuardrail{denyAll}},
	}, agents.RunOptions{Input: "hi"})
	if !errors.Is(err, agents.ErrGuardrailTripped) {
		t.Fatalf("err = %v, want ErrGuardrailTripped", err)
	}
}

func TestRunner_outputGuardrailTrips(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(textCompletion("forbidden answer"))
	})
	defer srv.Close()

	denyOutput := agents.OutputGuardrail{
		Name: "deny-output",
		Run:  func(_ context.Context, _ string) error { return errors.New("blocked") },
	}
	r := &agents.Runner{Client: c}
	_, err := r.Run(context.Background(), &agents.Agent{
		Name:       "Guarded",
		Model:      "m",
		Guardrails: agents.Guardrails{Output: []agents.OutputGuardrail{denyOutput}},
	}, agents.RunOptions{Input: "hi"})
	if !errors.Is(err, agents.ErrGuardrailTripped) {
		t.Fatalf("err = %v, want ErrGuardrailTripped", err)
	}
}

func TestRunner_handoffSwapsAgent(t *testing.T) {
	calls := 0
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			// First agent invokes handoff
			json.NewEncoder(w).Encode(toolCompletion("h1", "to_specialist", `{"reason":"need expert"}`))
		case 2:
			// Second agent gets the call, verify it sees the tool/result pair
			var req chat.CreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			lastSystem := ""
			for _, m := range req.Messages {
				if m.Role == "system" {
					if s, ok := m.Content.(string); ok {
						lastSystem = s
					}
				}
			}
			if lastSystem != "Specialist instructions" {
				t.Errorf("turn 2 should use specialist's instructions; got %q", lastSystem)
			}
			json.NewEncoder(w).Encode(textCompletion("specialist answer"))
		}
	})
	defer srv.Close()

	specialist := &agents.Agent{
		Name:         "Specialist",
		Model:        "m",
		Instructions: "Specialist instructions",
	}
	root := &agents.Agent{
		Name:         "Triage",
		Model:        "m",
		Instructions: "Triage instructions",
		Handoffs: []agents.Handoff{{
			Name:        "to_specialist",
			Description: "Handoff to the specialist",
			Agent:       specialist,
		}},
	}

	r := &agents.Runner{Client: c}
	res, err := r.Run(context.Background(), root, agents.RunOptions{Input: "help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.LastAgent.Name != "Specialist" {
		t.Errorf("LastAgent = %q, want Specialist", res.LastAgent.Name)
	}
	if res.Output != "specialist answer" {
		t.Errorf("Output = %q", res.Output)
	}
	// verify handoff was traced
	if res.Trajectory.Turns[0].Handoff == nil {
		t.Fatal("expected HandoffTrace on turn 0")
	}
	h := res.Trajectory.Turns[0].Handoff
	if h.From != "Triage" || h.To != "Specialist" {
		t.Errorf("handoff = %+v", h)
	}
	// turn 1 should show specialist as AgentName
	if res.Trajectory.Turns[1].AgentName != "Specialist" {
		t.Errorf("turn 1 agent = %q, want Specialist", res.Trajectory.Turns[1].AgentName)
	}
}

func TestRunner_hooksInvoked(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(textCompletion("ok"))
	})
	defer srv.Close()

	var startCalls, endCalls atomic.Int32
	r := &agents.Runner{
		Client: c,
		Hooks: agents.RunHooks{
			OnTurnStart: func(_ context.Context, _ int, _ *agents.Agent, _ []chat.Message) {
				startCalls.Add(1)
			},
			OnTurnEnd: func(_ context.Context, _ int, _ *chat.Completion) {
				endCalls.Add(1)
			},
		},
	}
	_, err := r.Run(context.Background(), &agents.Agent{Name: "X", Model: "m"}, agents.RunOptions{Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if startCalls.Load() != 1 || endCalls.Load() != 1 {
		t.Errorf("hooks: start=%d end=%d, want 1/1", startCalls.Load(), endCalls.Load())
	}
}

func TestRunner_middlewareWrapsRun(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(textCompletion("inner"))
	})
	defer srv.Close()

	mw := func(next agents.RunFunc) agents.RunFunc {
		return func(ctx context.Context, a *agents.Agent, opts agents.RunOptions) (*agents.RunResult, error) {
			res, err := next(ctx, a, opts)
			if err != nil {
				return nil, err
			}
			res.Output = "[wrapped] " + res.Output
			return res, nil
		}
	}
	r := &agents.Runner{Client: c, Middleware: []agents.Middleware{mw}}
	res, err := r.Run(context.Background(), &agents.Agent{Name: "X", Model: "m"}, agents.RunOptions{Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "[wrapped] inner" {
		t.Errorf("output = %q", res.Output)
	}
}

func TestRunner_ctxCancellation(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled
	r := &agents.Runner{Client: c}
	_, err := r.Run(ctx, &agents.Agent{Name: "X", Model: "m"}, agents.RunOptions{Input: "hi"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRunner_sessionPersistsAcrossRuns(t *testing.T) {
	calls := 0
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req chat.CreateRequest
		json.NewDecoder(r.Body).Decode(&req)
		switch calls {
		case 1:
			// 1st run: system + user("first")
			if got := len(req.Messages); got != 2 {
				t.Errorf("1st run: want 2 messages, got %d", got)
			}
			json.NewEncoder(w).Encode(textCompletion("ack first"))
		case 2:
			// 2nd run: system + user("first") + assistant("ack first") + user("second")
			if got := len(req.Messages); got != 4 {
				t.Errorf("2nd run: want 4 messages, got %d (%+v)", got, req.Messages)
			}
			json.NewEncoder(w).Encode(textCompletion("ack second"))
		}
	})
	defer srv.Close()

	sess := agents.NewInMemorySession("test")
	r := &agents.Runner{Client: c}
	a := &agents.Agent{Name: "X", Model: "m", Instructions: "you are x"}

	if _, err := r.Run(context.Background(), a, agents.RunOptions{Input: "first", Session: sess}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), a, agents.RunOptions{Input: "second", Session: sess}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionFromConversation_round_trip(t *testing.T) {
	srv, c := newChatClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(textCompletion("hi"))
	})
	defer srv.Close()

	conv := chat.NewConversation(c, "m")
	sess := agents.SessionFromConversation("conv", conv)

	if err := sess.Add(context.Background(), []chat.Message{{Role: "user", Content: "first"}}); err != nil {
		t.Fatal(err)
	}
	got, err := sess.Items(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Content != "first" {
		t.Errorf("items = %+v", got)
	}
	popped, _ := sess.Pop(context.Background())
	if popped == nil || popped.Content != "first" {
		t.Errorf("popped = %+v", popped)
	}
	got, _ = sess.Items(context.Background(), 0)
	if len(got) != 0 {
		t.Errorf("after pop, expected empty, got %+v", got)
	}
}

func TestInMemorySession_basics(t *testing.T) {
	s := agents.NewInMemorySession("")
	if s.ID() != "in-memory" {
		t.Errorf("default id = %q", s.ID())
	}
	ctx := context.Background()
	_ = s.Add(ctx, []chat.Message{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
	})
	got, _ := s.Items(ctx, 2)
	if len(got) != 2 || got[0].Content != "b" || got[1].Content != "c" {
		t.Errorf("limit=2 = %+v", got)
	}
	got, _ = s.Items(ctx, 0)
	if len(got) != 3 {
		t.Errorf("limit=0 should return all, got %d", len(got))
	}
	popped, _ := s.Pop(ctx)
	if popped.Content != "c" {
		t.Errorf("pop = %+v", popped)
	}
	_ = s.Clear(ctx)
	got, _ = s.Items(ctx, 0)
	if len(got) != 0 {
		t.Errorf("after clear: %+v", got)
	}
}
