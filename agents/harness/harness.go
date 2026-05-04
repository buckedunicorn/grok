// Package harness is the opinionated default agent on top of agents.Runner.
//
// A Harness pre-wires four tools that are useful across most coding/research
// agent workloads:
//
//   - Planning , write_todos / list_todos for task breakdown and progress tracking
//   - FS       , read_file / write_file / edit_file / list / glob / grep
//   - Shell    , execute (runs commands via a pluggable Executor)
//   - Subagent , task (spawns a child Runner with isolated Session)
//
// All four are opt-in through Options. NewDefault enables planning, an
// in-memory FS, and the subagent tool but NOT shell, shell access has
// the largest blast radius and should be enabled deliberately. The default
// Executor (LocalExecutor) runs commands directly on the host with no
// sandbox; production workloads should plug in a Docker, Firecracker, or
// other sandboxing Executor before enabling shell.
//
// Inspired by Deep Agents (langchain-ai/deepagents).
package harness

import (
	"context"
	"errors"

	"github.com/buckedunicorn/grok/agents"
	"github.com/buckedunicorn/grok/chat"
)

// Harness composes the default toolset on top of agents.Runner.
// Each call to Run uses a fresh State so todos and the in-memory FS do not
// leak between runs.
type Harness struct {
	Client       *chat.Client
	Model        string
	Instructions string
	MaxTurns     int
	Hooks        agents.RunHooks
	Middleware   []agents.Middleware
	Checkpointer agents.Checkpointer

	enablePlanning bool
	fs             FS
	executor       Executor
	enableSubagent bool
	subagentDepth  int // 0 = enabled; -1 = forbidden (used internally for child runs)
}

// Option configures a Harness.
type Option func(*Harness)

// WithPlanning enables the planning tools (write_todos, list_todos).
// Enabled by default in NewDefault.
func WithPlanning(enable bool) Option {
	return func(h *Harness) { h.enablePlanning = enable }
}

// WithFS installs a filesystem implementation. Pass nil to disable FS tools.
// NewDefault uses a MemoryFS.
func WithFS(fs FS) Option {
	return func(h *Harness) { h.fs = fs }
}

// WithExecutor installs a shell Executor. Pass nil to disable the shell tool.
// NewDefault leaves shell disabled.
func WithExecutor(e Executor) Option {
	return func(h *Harness) { h.executor = e }
}

// WithSubagent enables or disables the task tool that spawns child runs.
// Enabled by default in NewDefault.
func WithSubagent(enable bool) Option {
	return func(h *Harness) { h.enableSubagent = enable }
}

// WithInstructions overrides the default system prompt.
func WithInstructions(s string) Option {
	return func(h *Harness) { h.Instructions = s }
}

// WithMaxTurns sets the Runner's MaxTurns.
func WithMaxTurns(n int) Option {
	return func(h *Harness) { h.MaxTurns = n }
}

// WithHooks installs RunHooks on the underlying Runner.
func WithHooks(hooks agents.RunHooks) Option {
	return func(h *Harness) { h.Hooks = hooks }
}

// WithMiddleware appends Middleware to the underlying Runner.
func WithMiddleware(mw ...agents.Middleware) Option {
	return func(h *Harness) { h.Middleware = append(h.Middleware, mw...) }
}

// WithCheckpointer installs a Checkpointer on the underlying Runner so that
// every tool-call turn is persisted. Pair with Harness.Resume (or
// durable.Resume) to continue interrupted runs.
func WithCheckpointer(c agents.Checkpointer) Option {
	return func(h *Harness) { h.Checkpointer = c }
}

// NewDefault returns a Harness with planning + in-memory FS + subagent tools
// enabled and shell disabled. Pass options to override.
func NewDefault(client *chat.Client, model string, opts ...Option) *Harness {
	h := &Harness{
		Client:         client,
		Model:          model,
		Instructions:   DefaultInstructions,
		enablePlanning: true,
		fs:             NewMemoryFS(),
		enableSubagent: true,
	}
	for _, o := range opts {
		o(h)
	}
	return h
}

// Run executes the task on a fresh State and returns the structured RunResult.
// The caller's task string becomes the user input; instructions and tools
// are wired automatically per the Harness configuration.
func (h *Harness) Run(ctx context.Context, task string) (*agents.RunResult, error) {
	return h.execute(ctx, agents.RunOptions{Input: task})
}

// Resume continues a previously checkpointed run. The caller passes the
// CheckpointState (typically loaded from the Harness's Checkpointer or a
// durable.Store). Tools are rewired exactly as they would be on a fresh
// Run, Handlers aren't serialized, so the Harness configuration must
// match what was active when the checkpoint was saved.
func (h *Harness) Resume(ctx context.Context, state *agents.CheckpointState) (*agents.RunResult, error) {
	if state == nil {
		return nil, errors.New("harness: Resume state is nil")
	}
	return h.execute(ctx, agents.RunOptions{Resume: state})
}

func (h *Harness) execute(ctx context.Context, opts agents.RunOptions) (*agents.RunResult, error) {
	if h.Client == nil {
		return nil, errors.New("harness: Client is nil")
	}
	if h.Model == "" {
		return nil, errors.New("harness: Model is empty")
	}

	state := newState()
	tools := h.buildTools(state)

	agent := &agents.Agent{
		Name:         "Harness",
		Instructions: h.Instructions,
		Model:        h.Model,
		Tools:        tools,
	}

	runner := &agents.Runner{
		Client:       h.Client,
		MaxTurns:     h.MaxTurns,
		Hooks:        h.Hooks,
		Middleware:   h.Middleware,
		Checkpointer: h.Checkpointer,
	}

	return runner.Run(ctx, agent, opts)
}

// buildTools assembles the tool list per the Harness configuration. Tools
// close over the per-run state so todos and the in-memory FS scope to one Run.
func (h *Harness) buildTools(state *State) []agents.Tool {
	var tools []agents.Tool
	if h.enablePlanning {
		tools = append(tools, planningTools(state)...)
	}
	if h.fs != nil {
		tools = append(tools, fsTools(h.fs)...)
	}
	if h.executor != nil {
		tools = append(tools, shellTool(h.executor))
	}
	// subagentDepth == -1 is set internally by the subagent tool when spawning
	// a child Harness, to prevent unbounded recursion.
	if h.enableSubagent && h.subagentDepth >= 0 {
		tools = append(tools, subagentTool(h))
	}
	return tools
}
