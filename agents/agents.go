// Package agents is the harness layer over chat.Client.
//
// The package provides composable primitives, Agent, Tool, Handoff, Session,
// Guardrails, RunHooks, and a Runner that executes the tool-call loop while
// capturing a structured Trajectory of every turn.
//
// For a procedural one-shot tool-call loop with no trajectory or handoffs,
// use chat.Client.RunAgent directly. For multi-agent flows, guardrails,
// observable trajectories, or middleware, use Runner.
//
// The design mirrors openai-agents-python (the dominant 2026 convention)
// with idiomatic Go differences:
//
//   - Agent is a value, not an object you mutate
//   - Runner is the orchestrator; it holds policy (max turns, hooks, middleware)
//   - Trajectory captures per-turn state so downstream tooling can analyze runs
package agents

import (
	"context"
	"errors"

	"github.com/buckedunicorn/grok/chat"
)

// Handler is called when the model requests a tool invocation. argsJSON is
// the JSON-encoded arguments string from the model. The return value must be
// valid JSON (string, object, or array). On error, the error message is sent
// back to the model as the tool result.
type Handler func(ctx context.Context, argsJSON string) (string, error)

// Tool describes a callable function available to an Agent.
//
// Parameters is a JSON Schema object, e.g.:
//
//	map[string]any{
//	    "type": "object",
//	    "properties": map[string]any{
//	        "city": map[string]any{"type": "string"},
//	    },
//	    "required": []string{"city"},
//	}
type Tool struct {
	Name        string
	Description string
	Parameters  any
	Handler     Handler
}

// Handoff lets an Agent transfer control to a different Agent mid-run.
// At runtime the Runner exposes Handoff to the model as a Tool whose
// invocation swaps the active Agent and feeds the model's stated reason
// forward as the next turn's input.
type Handoff struct {
	// Name is the tool name the model invokes to trigger this handoff.
	Name string
	// Description is shown to the model so it knows when to hand off.
	Description string
	// Agent is the destination agent.
	Agent *Agent
}

// InputGuardrail validates the user input before the first turn. Return a
// non-nil error to abort the run with ErrGuardrailTripped wrapping it.
type InputGuardrail struct {
	Name string
	Run  func(ctx context.Context, input string) error
}

// OutputGuardrail validates each assistant turn's text output. Return a
// non-nil error to abort the run with ErrGuardrailTripped wrapping it.
type OutputGuardrail struct {
	Name string
	Run  func(ctx context.Context, output string) error
}

// Guardrails groups input and output checks that wrap a run.
type Guardrails struct {
	Input  []InputGuardrail
	Output []OutputGuardrail
}

// ModelOptions captures per-run knobs. Zero values are omitted.
type ModelOptions struct {
	Temperature       *float64
	MaxTokens         *int
	ReasoningEffort   string // "low" | "high" for reasoning models
	ResponseFormat    *chat.ResponseFormat
	ParallelToolCalls *bool
}

// Agent is a configured LLM persona with tools, optional handoffs, and
// optional guardrails. Agent is plain data, Runners consume it.
type Agent struct {
	Name         string
	Instructions string
	Model        string
	Tools        []Tool
	Handoffs     []Handoff
	Guardrails   Guardrails
	ModelOptions ModelOptions
}

// ErrMaxTurnsExceeded is returned when Runner.Run hits its turn limit
// without the model finishing.
var ErrMaxTurnsExceeded = errors.New("agents: runner exceeded maximum turns")

// ErrGuardrailTripped wraps an underlying guardrail error. Callers can use
// errors.Is to detect a guardrail trip and errors.As to extract details.
var ErrGuardrailTripped = errors.New("agents: guardrail tripped")
