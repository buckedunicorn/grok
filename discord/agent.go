package discord

import (
	"context"
	"sync"

	"github.com/buckedunicorn/grok/chat"
)

// AgentConfig configures an Agent.
type AgentConfig struct {
	// Model is the chat model used for every turn (e.g.
	// "grok-4-1-fast-non-reasoning").
	Model string

	// System is the system prompt prepended to every turn. Stored
	// outside per-channel history so it stays constant across resets.
	System string

	// MaxHistory is the max number of user/assistant turn pairs kept
	// per channel. Oldest pairs are dropped after each turn when
	// exceeded. 0 = unlimited.
	MaxHistory int

	// Tools is the list of function tools advertised to the model on
	// every turn. Pair with Handlers; the handler key must match
	// Tools[i].Function.Name. Both can be empty for a tool-free agent,
	// but at that point Router is lighter.
	Tools []chat.Tool

	// Handlers maps tool name → handler. Handlers receive a context
	// pre-populated with the channel ID via WithChannelContext, so
	// helpers like WebSearchTool can post acks without per-channel
	// closures.
	Handlers map[string]chat.Handler

	// MaxTurns caps how many tool-call rounds chat.RunAgent may take
	// in a single Handle call. 0 → 8 (deliberately tighter than
	// chat.RunAgent's default of 10, since Discord turns should feel
	// snappy).
	MaxTurns int
}

// Agent is the tool-aware sibling of Router. It maintains per-channel
// chat history (locked per channel, like Router) and runs each turn
// through chat.RunAgent so the model can call function tools mid-turn.
//
// Concurrent Handle calls on different channels run in parallel;
// concurrent calls on the same channel serialize.
//
// History stored: only the user message and the final assistant text.
// Intermediate tool_calls / tool_result messages are NOT persisted
// across turns, the assistant's textual reply already mentions any
// generated artifacts (URLs etc.), so future turns can refer back to
// them via the assistant's words alone. This avoids orphan-tool-call
// errors when trimming.
type Agent struct {
	client *chat.Client
	cfg    AgentConfig

	mu    sync.Mutex
	convs map[string]*agentChannel
}

type agentChannel struct {
	lk       sync.Mutex
	messages []chat.Message
}

// NewAgent constructs an Agent backed by client.
func NewAgent(client *chat.Client, cfg AgentConfig) *Agent {
	return &Agent{
		client: client,
		cfg:    cfg,
		convs:  make(map[string]*agentChannel),
	}
}

// Handle runs one turn for channelID with the given user message.
// Returns the assistant's final text reply. See HandleParts for
// vision-capable input.
func (a *Agent) Handle(ctx context.Context, channelID, userMessage string) (string, error) {
	return a.handle(ctx, channelID, userMessage)
}

// HandleParts is the multi-part variant of Handle. Pass a slice of
// chat.ContentPart (text + image_url entries) for vision-capable
// models. Per-channel routing, locking, and history trimming are
// identical to Handle.
func (a *Agent) HandleParts(ctx context.Context, channelID string, parts []chat.ContentPart) (string, error) {
	return a.handle(ctx, channelID, parts)
}

func (a *Agent) handle(ctx context.Context, channelID string, userContent any) (string, error) {
	ch := a.channel(channelID)
	ch.lk.Lock()
	defer ch.lk.Unlock()

	ch.messages = append(ch.messages, chat.Message{Role: "user", Content: userContent})

	msgs := make([]chat.Message, 0, len(ch.messages)+1)
	if a.cfg.System != "" {
		msgs = append(msgs, chat.Message{Role: "system", Content: a.cfg.System})
	}
	msgs = append(msgs, ch.messages...)

	maxTurns := a.cfg.MaxTurns
	if maxTurns == 0 {
		maxTurns = 8
	}

	// Thread channelID onto ctx so tool handlers built by this package
	// (and any user handlers that adopt the same convention) can find
	// it without a per-channel closure.
	ctx = WithChannelContext(ctx, channelID)

	comp, err := a.client.RunAgent(ctx, &chat.CreateRequest{
		Model:      a.cfg.Model,
		Messages:   msgs,
		Tools:      a.cfg.Tools,
		ToolChoice: toolChoice(a.cfg.Tools),
	}, a.cfg.Handlers, chat.WithMaxTurns(maxTurns))
	if err != nil {
		// Roll the user message back so retries don't compound.
		ch.messages = ch.messages[:len(ch.messages)-1]
		return "", err
	}

	answer := ""
	if len(comp.Choices) > 0 {
		answer, _ = comp.Choices[0].Message.Content.(string)
	}
	ch.messages = append(ch.messages, chat.Message{Role: "assistant", Content: answer})
	ch.messages = trimHistoryUserAligned(ch.messages, a.cfg.MaxHistory)
	return answer, nil
}

// Reset clears history for one channel. If a Handle call is in flight
// for the same channel, Reset blocks until it completes; this avoids
// silently losing the in-flight reply by orphaning the channel state
// it was about to write back.
func (a *Agent) Reset(channelID string) {
	a.mu.Lock()
	ch, ok := a.convs[channelID]
	a.mu.Unlock()
	if !ok {
		return
	}
	// Acquire the per-channel lock to wait for any in-flight Handle to
	// finish. After we hold lk, no other goroutine can be writing to
	// ch.messages; the in-flight Handle's append+trim cycle has
	// completed. Then drop the channel from the map. The next Handle
	// for this channel creates a fresh agentChannel with empty state.
	ch.lk.Lock()
	a.mu.Lock()
	delete(a.convs, channelID)
	a.mu.Unlock()
	ch.lk.Unlock()
}

// ResetAll clears history for every channel. Like Reset, ResetAll
// waits for every in-flight Handle to finish before clearing.
func (a *Agent) ResetAll() {
	a.mu.Lock()
	channels := make([]*agentChannel, 0, len(a.convs))
	for _, ch := range a.convs {
		channels = append(channels, ch)
	}
	a.mu.Unlock()

	for _, ch := range channels {
		ch.lk.Lock()
	}
	a.mu.Lock()
	a.convs = make(map[string]*agentChannel)
	a.mu.Unlock()
	for _, ch := range channels {
		ch.lk.Unlock()
	}
}

// Messages returns a snapshot of the channel's history (user/assistant
// only, the system prompt is not included). Returns nil when no Handle
// has run for channelID yet; the read does not insert an empty entry
// into the per-channel map.
func (a *Agent) Messages(channelID string) []chat.Message {
	a.mu.Lock()
	ch, ok := a.convs[channelID]
	a.mu.Unlock()
	if !ok {
		return nil
	}
	ch.lk.Lock()
	defer ch.lk.Unlock()
	out := make([]chat.Message, len(ch.messages))
	copy(out, ch.messages)
	return out
}

func (a *Agent) channel(channelID string) *agentChannel {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ch, ok := a.convs[channelID]; ok {
		return ch
	}
	ch := &agentChannel{}
	a.convs[channelID] = ch
	return ch
}

// toolChoice picks "auto" when tools are advertised and leaves it nil
// otherwise (xAI rejects tool_choice without tools).
func toolChoice(tools []chat.Tool) any {
	if len(tools) == 0 {
		return nil
	}
	return "auto"
}

// trimHistoryUserAligned keeps at most 2*maxPairs messages, preferring
// tails that start at a user message so we never leave a dangling
// assistant or tool message at the head. maxPairs ≤ 0 → no trim.
func trimHistoryUserAligned(msgs []chat.Message, maxPairs int) []chat.Message {
	if maxPairs <= 0 {
		return msgs
	}
	keep := maxPairs * 2
	if len(msgs) <= keep {
		return msgs
	}
	start := len(msgs) - keep
	for start < len(msgs) && msgs[start].Role != "user" {
		start++
	}
	if start >= len(msgs) {
		return msgs[len(msgs)-keep:]
	}
	return msgs[start:]
}
