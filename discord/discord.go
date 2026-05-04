// Package discord provides Discord-compatible patterns for the Grok SDK.
//
// Rather than importing a Discord client library (which would add a heavy
// transitive dependency), this package defines the minimal interfaces and
// helper types you need to wire up any Discord library, or your own HTTP
// handler, to the Grok chat API.
//
// # Pick a runtime
//
//   - Router: tool-free per-channel chat. Lightest option; backed by
//     chat.Conversation. Use for plain conversational bots.
//
//   - Agent: tool-aware per-channel chat backed by chat.RunAgent. Use
//     when you want the model to call function tools mid-turn (image /
//     video generation, server-side web_search, custom functions).
//     Agent.Handle threads channelID onto context.Context so tool
//     handlers can find it via ChannelFromContext.
//
// # Sender, the Discord-write surface
//
// Helpers that need to push messages back to Discord (KeepTyping,
// WebSearchTool, RunCodeTool, …) take a Sender, a two-method
// interface (Send, Typing) you implement with a thin adapter over
// discordgo, disgord, raw HTTP, or whatever you use. That's how the
// package stays free of any Discord-library dependency.
//
// # Pre-wired tools
//
// WebSearchTool and RunCodeTool return (chat.Tool, chat.Handler) pairs
// you can plug straight into AgentConfig.Tools / Handlers. They run
// xAI's server-side web_search / x_search / code_interpreter via the
// Responses API, and post a brief progress message to the Discord
// channel before each subcall when given a Sender.
//
// # Helpers
//
// KeepTyping refreshes Discord's "is typing…" indicator across long
// agent turns. Mention formats a user @-mention. LooksLikeURL,
// LooksLikeImageURL, LooksLikeVideoURL, ExtractURLs are conservative
// classifiers for user-pasted or model-emitted links. Chunks splits a
// reply across Discord's 2000-char per-message cap.
//
// Both Router and Agent are safe for concurrent use across channels.
// Concurrent calls on the SAME channelID serialize on a per-channel
// mutex, the channel's chat history isn't shared between goroutines.
package discord

import (
	"context"
	"sync"

	"github.com/buckedunicorn/grok/chat"
)

// RouterConfig configures a Router.
type RouterConfig struct {
	// Model is the Grok model to use (e.g. "grok-4-1-fast-non-reasoning").
	Model string
	// System is the system prompt injected into every conversation.
	System string
	// MaxHistory is the max number of user+assistant turn pairs kept per
	// channel. Oldest pairs are dropped when exceeded. 0 means unlimited.
	MaxHistory int
}

// Router manages per-channel chat.Conversations and routes Discord messages
// to Grok, returning the reply text. Wire it into whichever Discord library
// you use, it has no external dependencies.
type Router struct {
	client *chat.Client
	cfg    RouterConfig

	mu    sync.Mutex // guards convs map only
	convs map[string]*channel
}

// channel pairs a per-channel Conversation with the lock that serializes
// Handle calls for that channel. chat.Conversation is not safe for
// concurrent use, so we hold lk for the duration of every Handle call on
// the channel.
type channel struct {
	lk   sync.Mutex
	conv *chat.Conversation
}

// NewRouter creates a Router backed by client.
func NewRouter(client *chat.Client, cfg RouterConfig) *Router {
	return &Router{
		client: client,
		cfg:    cfg,
		convs:  make(map[string]*channel),
	}
}

// Handle sends userMessage from channelID to Grok and returns the reply.
// It creates a new per-channel Conversation on first call and maintains
// history across calls for the same channelID. Concurrent calls on the
// same channelID are serialized; calls on different channels run in
// parallel.
func (r *Router) Handle(ctx context.Context, channelID, userMessage string) (string, error) {
	ch := r.channel(channelID)
	ch.lk.Lock()
	defer ch.lk.Unlock()

	comp, err := ch.conv.Send(ctx, userMessage)
	if err != nil {
		return "", err
	}
	return r.replyFromCompletion(ch, comp), nil
}

// HandleParts is the multi-part variant of Handle. Pass a slice of
// chat.ContentPart (text + image_url entries) for vision-capable models;
// per-channel routing, locking, and history trimming are identical to
// Handle. Returns the assistant reply text.
func (r *Router) HandleParts(ctx context.Context, channelID string, parts []chat.ContentPart) (string, error) {
	ch := r.channel(channelID)
	ch.lk.Lock()
	defer ch.lk.Unlock()

	comp, err := ch.conv.SendParts(ctx, parts)
	if err != nil {
		return "", err
	}
	return r.replyFromCompletion(ch, comp), nil
}

func (r *Router) replyFromCompletion(ch *channel, comp *chat.Completion) string {
	reply := ""
	if len(comp.Choices) > 0 {
		reply, _ = comp.Choices[0].Message.Content.(string)
	}
	r.trimHistory(ch)
	return reply
}

// Reset clears the conversation history for channelID.
func (r *Router) Reset(channelID string) {
	r.mu.Lock()
	delete(r.convs, channelID)
	r.mu.Unlock()
}

// ResetAll clears conversation history for every channel.
func (r *Router) ResetAll() {
	r.mu.Lock()
	r.convs = make(map[string]*channel)
	r.mu.Unlock()
}

// Chunks splits text into Discord-safe chunks (≤ maxLen characters each).
// maxLen ≤ 0 defaults to 2000 (Discord's per-message limit).
// Pass the result directly to your Discord library's send function.
func Chunks(text string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = 2000
	}
	if len(text) <= maxLen {
		return []string{text}
	}
	var out []string
	for len(text) > 0 {
		n := min(maxLen, len(text))
		out = append(out, text[:n])
		text = text[n:]
	}
	return out
}

func (r *Router) channel(channelID string) *channel {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ch, ok := r.convs[channelID]; ok {
		return ch
	}
	opts := []chat.ConvOption{}
	if r.cfg.System != "" {
		opts = append(opts, chat.WithSystem(r.cfg.System))
	}
	ch := &channel{conv: chat.NewConversation(r.client, r.cfg.Model, opts...)}
	r.convs[channelID] = ch
	return ch
}

// trimHistory drops the oldest user+assistant pairs in place when the
// channel's conversation exceeds MaxHistory turns. Caller must hold ch.lk.
//
// Earlier versions rebuilt the Conversation by re-Sending every retained
// user message, that burned a Send per pair AND replaced real assistant
// replies with regenerated text. AppendMessages on a fresh Conversation
// preserves history without any API calls.
func (r *Router) trimHistory(ch *channel) {
	if r.cfg.MaxHistory <= 0 {
		return
	}
	msgs := ch.conv.Messages()
	keep := r.cfg.MaxHistory * 2
	if len(msgs) <= keep {
		return
	}
	tail := msgs[len(msgs)-keep:]
	opts := []chat.ConvOption{}
	if r.cfg.System != "" {
		opts = append(opts, chat.WithSystem(r.cfg.System))
	}
	fresh := chat.NewConversation(r.client, r.cfg.Model, opts...)
	fresh.AppendMessages(tail...)
	ch.conv = fresh
}
