package agents

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/buckedunicorn/grok/chat"
)

// Runner executes the tool-call loop for an Agent and produces a structured
// Trajectory of every turn. It is the orchestrator; Agent is the data.
//
// A Runner is safe for concurrent use by multiple goroutines as long as
// each Run uses its own Session.
type Runner struct {
	// Client is the underlying chat client. Required.
	Client *chat.Client
	// MaxTurns caps tool-call rounds. 0 means use defaultMaxTurns (10).
	MaxTurns int
	// Hooks receive lifecycle callbacks. Zero value disables hooks.
	Hooks RunHooks
	// Middleware wraps each Run call. chain[0] runs first; chain[len-1]
	// is closest to the underlying loop.
	Middleware []Middleware
	// Checkpointer, when non-nil, persists CheckpointState after every
	// turn. A SaveCheckpoint error aborts the run. See agents/durable for
	// production-grade implementations (FileStore, MemoryStore).
	Checkpointer Checkpointer

	// chatToolsMu and chatToolsCache memoize the per-Agent chat.Tool
	// slice. Agent.Tools and Agent.Handoffs are documented as
	// immutable, so we convert once and reuse across every turn
	//.
	chatToolsMu    sync.Mutex
	chatToolsCache map[*Agent][]chat.Tool
}

// RunOptions parameterizes a single Run.
type RunOptions struct {
	// Input is the user message for the run. Empty Input is allowed if
	// Session already contains messages (e.g. a continuation) or Resume
	// is set.
	Input string
	// Session persists message history. Nil means a fresh in-memory
	// session. Cannot be combined with Resume, when resuming, the
	// Runner builds an InMemorySession from the saved Messages.
	Session Session
	// Resume, when non-nil, continues a previously checkpointed run. The
	// Runner picks up at state.NextTurn with state.Messages as the session
	// history and state.Trajectory as the accumulated trajectory. The
	// caller must pass the Agent that was active at checkpoint time
	// (state.AgentName is informational only, the Runner does not look
	// up the agent for you).
	Resume *CheckpointState
}

// RunResult is the structured output of Runner.Run.
//
// On error, Run still returns a non-nil RunResult whenever any meaningful
// state has been captured, Trajectory.Turns will hold every turn that
// completed before the failure, including the failing one. The Output
// field is empty in that case (no final answer was produced) but every
// other field is populated. This lets callers debug ErrMaxTurnsExceeded,
// ErrGuardrailTripped, and underlying chat-API errors against a real
// Trajectory instead of guessing what happened.
type RunResult struct {
	// Output is the final assistant text after the loop terminated.
	// Empty when Run returns an error.
	Output string
	// Messages is the full ordered transcript including system prompt,
	// the input, every assistant turn, every tool call, and every tool
	// result. Identical in shape to chat.RunAgent's output.
	Messages []chat.Message
	// Usage is the cumulative token usage across every turn in this run.
	Usage chat.Usage
	// LastAgent is the agent that produced the final turn. Equal to the
	// input agent unless a Handoff transferred control.
	LastAgent *Agent
	// Trajectory is the structured turn-by-turn record of this run.
	Trajectory Trajectory
}

// Trajectory is a structured trace of every turn the Runner executed.
// Designed so downstream tooling (offline analysis, eval harnesses,
// reflection loops) can index runs without reparsing the message stream.
type Trajectory struct {
	RunID     string
	StartedAt time.Time
	EndedAt   time.Time
	Turns     []Turn
}

// Turn captures everything that happened in a single Runner iteration.
//
// Memory note: Input historically held a fresh copy of
// the full system+history prefix sent to the model on this turn. With
// SnapshotSession (the default for InMemorySession), Input now aliases
// the session's underlying slice when no system prompt is configured.
// Treat Input as read-only; mutating it corrupts later turns. NewMessages
// is the recommended field for trace consumers that only want what this
// turn added.
type Turn struct {
	Index     int
	AgentName string
	// Input is the message slice sent to the model on this turn.
	// Read-only; may share storage with the session.
	Input []chat.Message
	// NewMessages contains the messages added during this turn (the
	// assistant message and any tool result messages). Populated even
	// when Input is shared.
	NewMessages []chat.Message
	Output      *chat.Completion
	ToolCalls   []ToolCallTrace
	Handoff     *HandoffTrace
	Guardrails  []GuardrailTrace
	Usage       chat.Usage
	DurationMS  int64
	Err         error
}

// ToolCallTrace captures a single tool invocation within a turn.
type ToolCallTrace struct {
	ID         string
	Name       string
	ArgsJSON   string
	Result     string
	Err        error
	DurationMS int64
}

// HandoffTrace records a control transfer to a different Agent.
type HandoffTrace struct {
	From, To string
	Reason   string
}

// GuardrailTrace records the result of a guardrail check.
type GuardrailTrace struct {
	Stage  string // "input" | "output"
	Name   string
	Passed bool
	Err    error
}

const defaultMaxTurns = 10

// Run executes the agent loop and returns a RunResult with a complete
// Trajectory. Run never mutates agent or opts.
func (r *Runner) Run(ctx context.Context, agent *Agent, opts RunOptions) (*RunResult, error) {
	base := r.runBase
	wrapped := chain(base, r.Middleware)
	return wrapped(ctx, agent, opts)
}

// runBase is the underlying loop, before middleware wrapping.
func (r *Runner) runBase(ctx context.Context, agent *Agent, opts RunOptions) (*RunResult, error) {
	if r.Client == nil {
		return nil, errors.New("agents: Runner.Client is nil")
	}
	if agent == nil {
		return nil, errors.New("agents: agent is nil")
	}
	if opts.Resume != nil && opts.Session != nil {
		return nil, errors.New("agents: RunOptions.Resume and Session are mutually exclusive")
	}

	maxTurns := r.MaxTurns
	if maxTurns <= 0 {
		maxTurns = defaultMaxTurns
	}

	session := opts.Session
	if session == nil {
		session = NewInMemorySession("")
	}

	traj := Trajectory{
		RunID:     newRunID(),
		StartedAt: time.Now(),
	}
	var totalUsage chat.Usage
	startTurn := 0

	// --- Resume path: seed everything from the checkpoint ---
	if opts.Resume != nil {
		traj.RunID = opts.Resume.RunID
		traj.StartedAt = opts.Resume.Trajectory.StartedAt
		traj.Turns = append(traj.Turns, opts.Resume.Trajectory.Turns...)
		totalUsage = opts.Resume.Usage
		startTurn = opts.Resume.NextTurn
		if err := session.Add(ctx, opts.Resume.Messages); err != nil {
			return nil, fmt.Errorf("agents: seed resumed session: %w", err)
		}
		// Skip input guardrails on resume, they ran against the original
		// Input on the first attempt; rerunning would be incorrect for
		// stateful guardrails.
		// Skip Input append, it's already in Messages.
	}

	// --- Input guardrails (only on fresh runs) ---
	var inputGuards []GuardrailTrace
	if opts.Resume == nil {
		for _, g := range agent.Guardrails.Input {
			t := GuardrailTrace{Stage: "input", Name: g.Name, Passed: true}
			if g.Run != nil {
				if err := g.Run(ctx, opts.Input); err != nil {
					t.Passed = false
					t.Err = err
					inputGuards = append(inputGuards, t)
					traj.EndedAt = time.Now()
					// Synthesize a turn-zero entry so the failed guardrail
					// trace shows up in the returned Trajectory.
					traj.Turns = append(traj.Turns, Turn{
						Index:      0,
						AgentName:  agent.Name,
						Guardrails: inputGuards,
						Err:        err,
					})
					return &RunResult{
						LastAgent:  agent,
						Trajectory: traj,
					}, fmt.Errorf("%w: %s: %w", ErrGuardrailTripped, g.Name, err)
				}
			}
			inputGuards = append(inputGuards, t)
		}

		// --- Seed the session with system + input ---
		if opts.Input != "" {
			if err := session.Add(ctx, []chat.Message{{Role: "user", Content: opts.Input}}); err != nil {
				return nil, fmt.Errorf("agents: session.Add: %w", err)
			}
		}
	}

	// --- Main loop ---
	current := agent

	for turn := startTurn; turn < maxTurns; turn++ {
		turnStart := time.Now()

		// Build the message slice we send to the model: system + session
		// items. Use Snapshot when the Session supports it to skip the
		// per-turn defensive copy. The Runner does not
		// mutate hist; it copies into the request slice via append.
		hist, err := sessionRead(ctx, session)
		if err != nil {
			return nil, fmt.Errorf("agents: session.Items: %w", err)
		}
		// When no system prompt is set, the request slice aliases the
		// session snapshot directly ( + ).
		// Otherwise we allocate the prefix+history once for this turn.
		var msgs []chat.Message
		if current.Instructions == "" {
			msgs = hist
		} else {
			msgs = make([]chat.Message, 0, len(hist)+1)
			msgs = append(msgs, chat.Message{Role: "system", Content: current.Instructions})
			msgs = append(msgs, hist...)
		}

		if r.Hooks.OnTurnStart != nil {
			r.Hooks.OnTurnStart(ctx, turn, current, msgs)
		}

		req := r.buildCreateRequest(current, msgs)
		comp, err := r.Client.Create(ctx, req)
		if err != nil {
			tr := Turn{
				Index:      turn,
				AgentName:  current.Name,
				Input:      msgs,
				DurationMS: time.Since(turnStart).Milliseconds(),
				Err:        err,
			}
			if turn == 0 {
				tr.Guardrails = inputGuards
			}
			traj.Turns = append(traj.Turns, tr)
			traj.EndedAt = time.Now()
			return &RunResult{
				Messages:   msgs,
				Usage:      totalUsage,
				LastAgent:  current,
				Trajectory: traj,
			}, err
		}

		totalUsage = addUsage(totalUsage, comp.Usage)

		if r.Hooks.OnTurnEnd != nil {
			r.Hooks.OnTurnEnd(ctx, turn, comp)
		}

		tr := Turn{
			Index:     turn,
			AgentName: current.Name,
			Input:     msgs,
			Output:    comp,
			Usage:     comp.Usage,
		}
		if turn == 0 {
			tr.Guardrails = inputGuards
		}

		if len(comp.Choices) == 0 {
			tr.DurationMS = time.Since(turnStart).Milliseconds()
			traj.Turns = append(traj.Turns, tr)
			traj.EndedAt = time.Now()
			return &RunResult{
				Messages:   msgs,
				Usage:      totalUsage,
				LastAgent:  current,
				Trajectory: traj,
			}, nil
		}

		choice := comp.Choices[0]

		// Terminal turn, model produced a final answer.
		if choice.FinishReason != "tool_calls" {
			finalText, _ := choice.Message.Content.(string)

			// Output guardrails.
			for _, g := range current.Guardrails.Output {
				gt := GuardrailTrace{Stage: "output", Name: g.Name, Passed: true}
				if g.Run != nil {
					if err := g.Run(ctx, finalText); err != nil {
						gt.Passed = false
						gt.Err = err
						tr.Guardrails = append(tr.Guardrails, gt)
						tr.DurationMS = time.Since(turnStart).Milliseconds()
						traj.Turns = append(traj.Turns, tr)
						traj.EndedAt = time.Now()
						return &RunResult{
							Messages:   msgs,
							Usage:      totalUsage,
							LastAgent:  current,
							Trajectory: traj,
						}, fmt.Errorf("%w: %s: %w", ErrGuardrailTripped, g.Name, err)
					}
				}
				tr.Guardrails = append(tr.Guardrails, gt)
			}

			if err := session.Add(ctx, []chat.Message{choice.Message}); err != nil {
				return nil, fmt.Errorf("agents: session.Add: %w", err)
			}
			tr.NewMessages = []chat.Message{choice.Message}
			tr.DurationMS = time.Since(turnStart).Milliseconds()
			traj.Turns = append(traj.Turns, tr)
			traj.EndedAt = time.Now()

			finalHist, _ := session.Items(ctx, 0)
			return &RunResult{
				Output:     finalText,
				Messages:   prependSystem(current.Instructions, finalHist),
				Usage:      totalUsage,
				LastAgent:  current,
				Trajectory: traj,
			}, nil
		}

		// Guard against a server bug where finish_reason="tool_calls"
		// arrives with no tool_calls. Re-sending the same conversation
		// would loop until maxTurns. Better to stop now and surface
		// the inconsistency.
		if len(choice.Message.ToolCalls) == 0 {
			tr.DurationMS = time.Since(turnStart).Milliseconds()
			tr.Err = errors.New("agents: server returned finish_reason=tool_calls with no tool_calls")
			traj.Turns = append(traj.Turns, tr)
			traj.EndedAt = time.Now()
			return &RunResult{
				Messages:   msgs,
				Usage:      totalUsage,
				LastAgent:  current,
				Trajectory: traj,
			}, tr.Err
		}

		// Tool-call turn, record assistant message, dispatch tools.
		if err := session.Add(ctx, []chat.Message{choice.Message}); err != nil {
			return nil, fmt.Errorf("agents: session.Add: %w", err)
		}
		tr.NewMessages = append(tr.NewMessages, choice.Message)

		var handoffTrace *HandoffTrace
		var nextAgent *Agent

		for _, tc := range choice.Message.ToolCalls {
			callStart := time.Now()
			if r.Hooks.OnToolCall != nil {
				r.Hooks.OnToolCall(ctx, turn, tc.Function.Name, tc.Function.Arguments)
			}

			// Resolve handoff first so it short-circuits regular tool dispatch.
			if h := findHandoff(current, tc.Function.Name); h != nil {
				reason := tc.Function.Arguments // model's stated reason as JSON
				handoffTrace = &HandoffTrace{From: current.Name, To: h.Agent.Name, Reason: reason}
				if r.Hooks.OnHandoff != nil {
					r.Hooks.OnHandoff(ctx, current, h.Agent, reason)
				}
				nextAgent = h.Agent
				// Synthesize a tool result confirming the handoff so the
				// new agent sees a complete tool/result pair in history.
				result := fmt.Sprintf(`{"handoff_to":%q}`, h.Agent.Name)
				toolMsg := chat.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result,
				}
				if err := session.Add(ctx, []chat.Message{toolMsg}); err != nil {
					return nil, fmt.Errorf("agents: session.Add: %w", err)
				}
				tr.NewMessages = append(tr.NewMessages, toolMsg)
				tr.ToolCalls = append(tr.ToolCalls, ToolCallTrace{
					ID:         tc.ID,
					Name:       tc.Function.Name,
					ArgsJSON:   redactArgs(ctx, r.Hooks, tc.Function.Name, tc.Function.Arguments),
					Result:     redactResult(ctx, r.Hooks, tc.Function.Name, result),
					DurationMS: time.Since(callStart).Milliseconds(),
				})
				if r.Hooks.OnToolResult != nil {
					r.Hooks.OnToolResult(ctx, turn, tc.Function.Name, result, nil)
				}
				continue
			}

			// Regular tool.
			result, terr := dispatchTool(ctx, current.Tools, tc)
			tct := ToolCallTrace{
				ID:         tc.ID,
				Name:       tc.Function.Name,
				ArgsJSON:   redactArgs(ctx, r.Hooks, tc.Function.Name, tc.Function.Arguments),
				Result:     redactResult(ctx, r.Hooks, tc.Function.Name, result),
				Err:        terr,
				DurationMS: time.Since(callStart).Milliseconds(),
			}
			tr.ToolCalls = append(tr.ToolCalls, tct)

			toolMsg := chat.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result,
			}
			if err := session.Add(ctx, []chat.Message{toolMsg}); err != nil {
				return nil, fmt.Errorf("agents: session.Add: %w", err)
			}
			tr.NewMessages = append(tr.NewMessages, toolMsg)

			if r.Hooks.OnToolResult != nil {
				r.Hooks.OnToolResult(ctx, turn, tc.Function.Name, result, terr)
			}
		}

		tr.Handoff = handoffTrace
		tr.DurationMS = time.Since(turnStart).Milliseconds()
		traj.Turns = append(traj.Turns, tr)

		if nextAgent != nil {
			current = nextAgent
		}

		// Checkpoint after each tool-call turn, terminal turns return
		// before reaching here, so checkpointing only happens when a
		// resumable continuation point exists.
		if r.Checkpointer != nil {
			hist, herr := session.Items(ctx, 0)
			if herr != nil {
				return nil, fmt.Errorf("agents: checkpoint snapshot session: %w", herr)
			}
			cp := CheckpointState{
				RunID:      traj.RunID,
				NextTurn:   turn + 1,
				AgentName:  current.Name,
				Messages:   hist,
				Trajectory: traj,
				Usage:      totalUsage,
				SavedAt:    time.Now(),
			}
			if err := r.Checkpointer.SaveCheckpoint(ctx, cp); err != nil {
				return nil, fmt.Errorf("agents: SaveCheckpoint: %w", err)
			}
		}
	}

	traj.EndedAt = time.Now()
	hist, _ := session.Items(ctx, 0)
	return &RunResult{
		Messages:   prependSystem(current.Instructions, hist),
		Usage:      totalUsage,
		LastAgent:  current,
		Trajectory: traj,
	}, ErrMaxTurnsExceeded
}

// buildCreateRequest packs an Agent + history into a chat.CreateRequest.
// Uses the Runner's chat.Tool cache so per-turn tool conversion is
// done once per Agent.
func (r *Runner) buildCreateRequest(agent *Agent, msgs []chat.Message) *chat.CreateRequest {
	tools := r.chatToolsFor(agent)
	req := &chat.CreateRequest{
		Model:    agent.Model,
		Messages: msgs,
		Tools:    tools,
	}
	if len(tools) > 0 {
		req.ToolChoice = "auto"
	}
	if agent.ModelOptions.Temperature != nil {
		req.Temperature = agent.ModelOptions.Temperature
	}
	if agent.ModelOptions.MaxTokens != nil {
		req.MaxCompletionTokens = agent.ModelOptions.MaxTokens
	}
	if agent.ModelOptions.ReasoningEffort != "" {
		req.ReasoningEffort = agent.ModelOptions.ReasoningEffort
	}
	if agent.ModelOptions.ResponseFormat != nil {
		req.ResponseFormat = agent.ModelOptions.ResponseFormat
	}
	if agent.ModelOptions.ParallelToolCalls != nil {
		req.ParallelToolCalls = agent.ModelOptions.ParallelToolCalls
	}
	return req
}

// chatToolsFor returns the cached chat.Tool slice for agent, computing
// it on first request. The result is shared across every turn of every
// concurrent Run that uses the same *Agent pointer.
func (r *Runner) chatToolsFor(agent *Agent) []chat.Tool {
	r.chatToolsMu.Lock()
	defer r.chatToolsMu.Unlock()
	if r.chatToolsCache == nil {
		r.chatToolsCache = make(map[*Agent][]chat.Tool)
	}
	if cached, ok := r.chatToolsCache[agent]; ok {
		return cached
	}
	tools := agentChatTools(agent)
	r.chatToolsCache[agent] = tools
	return tools
}

// agentChatTools converts an Agent's Tools and Handoffs into chat.Tool defs.
// Handoffs are exposed to the model as zero-arg function tools.
func agentChatTools(agent *Agent) []chat.Tool {
	if len(agent.Tools) == 0 && len(agent.Handoffs) == 0 {
		return nil
	}
	out := make([]chat.Tool, 0, len(agent.Tools)+len(agent.Handoffs))
	for _, t := range agent.Tools {
		out = append(out, chat.Tool{
			Type: "function",
			Function: chat.FunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	for _, h := range agent.Handoffs {
		out = append(out, chat.Tool{
			Type: "function",
			Function: chat.FunctionDef{
				Name:        h.Name,
				Description: h.Description,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"reason": map[string]any{
							"type":        "string",
							"description": "Why control is being handed off.",
						},
					},
				},
			},
		})
	}
	return out
}

// findHandoff returns the matching Handoff on agent or nil.
func findHandoff(agent *Agent, name string) *Handoff {
	for i := range agent.Handoffs {
		if agent.Handoffs[i].Name == name {
			return &agent.Handoffs[i]
		}
	}
	return nil
}

// dispatchTool finds the named Tool on agent.Tools and invokes its Handler.
// Unknown tools return a JSON error string. Handler errors return a JSON
// error string AND the original error so the trace records both.
func dispatchTool(ctx context.Context, tools []Tool, tc chat.ToolCall) (string, error) {
	for _, t := range tools {
		if t.Name == tc.Function.Name && t.Handler != nil {
			res, err := t.Handler(ctx, tc.Function.Arguments)
			if err != nil {
				return errorJSON(err.Error()), err
			}
			return res, nil
		}
	}
	return errorJSON("unknown tool " + tc.Function.Name),
		fmt.Errorf("unknown tool %q", tc.Function.Name)
}

// sessionRead returns the session's full history, preferring the
// SnapshotSession fast path (which avoids an internal copy) when the
// concrete implementation supports it.
func sessionRead(ctx context.Context, session Session) ([]chat.Message, error) {
	if snap, ok := session.(SnapshotSession); ok {
		return snap.Snapshot(ctx)
	}
	return session.Items(ctx, 0)
}

// errorJSON marshals msg into {"error":"..."}. Uses encoding/json so
// quotes, backslashes, control characters, and non-ASCII bytes are
// escaped per RFC 8259 rather than via Go's %q. Typed
// struct skips the map[string]string allocation per call.
func errorJSON(msg string) string {
	b, _ := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: msg})
	return string(b)
}

// redactArgs runs the optional Hooks.RedactToolArgs hook before the
// arguments string is recorded into the trajectory.
func redactArgs(ctx context.Context, hooks RunHooks, name, args string) string {
	if hooks.RedactToolArgs == nil {
		return args
	}
	return hooks.RedactToolArgs(ctx, name, args)
}

// redactResult runs the optional Hooks.RedactToolResult hook before
// the result string is recorded into the trajectory.
func redactResult(ctx context.Context, hooks RunHooks, name, result string) string {
	if hooks.RedactToolResult == nil {
		return result
	}
	return hooks.RedactToolResult(ctx, name, result)
}

// prependSystem returns a new slice with system instructions prepended if
// non-empty. Used to construct RunResult.Messages.
func prependSystem(instructions string, hist []chat.Message) []chat.Message {
	if instructions == "" {
		out := make([]chat.Message, len(hist))
		copy(out, hist)
		return out
	}
	out := make([]chat.Message, 0, len(hist)+1)
	out = append(out, chat.Message{Role: "system", Content: instructions})
	out = append(out, hist...)
	return out
}

// addUsage accumulates Usage values across turns.
func addUsage(a, b chat.Usage) chat.Usage {
	a.PromptTokens += b.PromptTokens
	a.CompletionTokens += b.CompletionTokens
	a.TotalTokens += b.TotalTokens
	a.PromptTokensDetails.CachedTokens += b.PromptTokensDetails.CachedTokens
	a.CompletionTokensDetails.ReasoningTokens += b.CompletionTokensDetails.ReasoningTokens
	return a
}

// newRunID returns a short hex id for a Trajectory.
func newRunID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
