package chat

import "context"

// Conversation manages a stateful multi-turn chat session for the Chat
// Completions API. Messages accumulate automatically; use Reset to start
// fresh.
//
// A Conversation is NOT safe for concurrent use. Each Conversation expects
// one Send / AppendMessages / PopMessage / Reset call at a time. Wrap
// access in your own lock if you need to share a Conversation across
// goroutines, or use one Conversation per goroutine. The discord.Router
// uses the latter strategy: one Conversation per channel, with a
// per-channel mutex serializing Handle calls on the same channel.
type Conversation struct {
	client   *Client
	model    string
	system   string
	messages []Message
}

// ConvOption configures a Conversation.
type ConvOption func(*Conversation)

// WithSystem sets a system prompt prepended to every request (not stored in history).
func WithSystem(s string) ConvOption {
	return func(c *Conversation) { c.system = s }
}

// NewConversation creates a multi-turn chat helper bound to client and model.
func NewConversation(c *Client, model string, opts ...ConvOption) *Conversation {
	cv := &Conversation{client: c, model: model}
	for _, o := range opts {
		o(cv)
	}
	// Seed the system message once so each Send does not re-prepend it
	//. Reset re-seeds via the same path.
	if cv.system != "" {
		cv.messages = []Message{{Role: "system", Content: cv.system}}
	}
	return cv
}

// Send appends a user message with string content, calls Create, stores the
// assistant reply, and returns the full Completion. On error the user
// message is rolled back so the history remains consistent.
func (cv *Conversation) Send(ctx context.Context, content string) (*Completion, error) {
	return cv.send(ctx, content)
}

// SendParts is the multi-part variant of Send. Pass a slice of ContentPart
// (e.g. text plus one or more image_url entries) for vision-enabled
// requests. The assistant reply is stored exactly the same way as for
// Send, only the user-side message content shape differs.
//
// Use it for vision-capable models when the user message includes images:
//
//	parts := []chat.ContentPart{
//	 {Type: "text", Text: "what's in this picture?"},
//	 {Type: "image_url", ImageURL: &chat.ImageURL{URL: "https://…/cat.png"}},
//	}
//	comp, err := conv.SendParts(ctx, parts)
func (cv *Conversation) SendParts(ctx context.Context, parts []ContentPart) (*Completion, error) {
	return cv.send(ctx, parts)
}

// send is the shared implementation for Send and SendParts. content must
// be a value json/Marshalable as a Message.Content field, a string or a
// []ContentPart, in practice.
//
// The system prompt (when set) lives at cv.messages[0] permanently, so
// each Send appends the user message and reuses the existing slice
// rather than building a new prepended copy per call.
func (cv *Conversation) send(ctx context.Context, content any) (*Completion, error) {
	cv.messages = append(cv.messages, Message{Role: "user", Content: content})

	comp, err := cv.client.Create(ctx, &CreateRequest{
		Model:    cv.model,
		Messages: cv.messages,
	})
	if err != nil {
		// Zero the slot before truncating so the failed user
		// content does not stay pinned in the backing array
		// indefinitely. Cheap; matters when Content
		// holds a large []ContentPart with image bytes etc.
		last := len(cv.messages) - 1
		cv.messages[last] = Message{}
		cv.messages = cv.messages[:last]
		return nil, err
	}

	if len(comp.Choices) > 0 {
		cv.messages = append(cv.messages, comp.Choices[0].Message)
	}
	return comp, nil
}

// Messages returns a snapshot of the conversation history, excluding the system prompt.
func (cv *Conversation) Messages() []Message {
	start := cv.userMessagesStart()
	out := make([]Message, len(cv.messages)-start)
	copy(out, cv.messages[start:])
	return out
}

// MessagesView returns the underlying message slice (excluding the
// system prompt) WITHOUT copying. The returned slice is read-only;
// mutating it corrupts the Conversation. Subsequent Send calls grow
// the slice and may invalidate the returned view via reslice.
//
// Use MessagesView when you only need to read the history and want
// to avoid the per-call copy that Messages does. The
// agents.SessionFromConversation adapter uses this on its per-turn
// read path.
func (cv *Conversation) MessagesView() []Message {
	return cv.messages[cv.userMessagesStart():]
}

func (cv *Conversation) userMessagesStart() int {
	if cv.system != "" && len(cv.messages) > 0 && cv.messages[0].Role == "system" {
		return 1
	}
	return 0
}

// Reset clears the conversation history. If a system prompt is
// configured, it is re-seeded as the first message (matching the
// invariant established by NewConversation, see ).
func (cv *Conversation) Reset() {
	if cv.system != "" {
		cv.messages = []Message{{Role: "system", Content: cv.system}}
		return
	}
	cv.messages = nil
}

// AppendMessages adds raw messages directly to the history without invoking
// the API. Useful for replaying transcripts or splicing tool calls/results.
// The agents package uses this via SessionFromConversation.
func (cv *Conversation) AppendMessages(items ...Message) {
	cv.messages = append(cv.messages, items...)
}

// PopMessage removes and returns the most recent message, or nil if
// empty. Refuses to pop the system message that keeps at
// index 0; returns nil in that case.
func (cv *Conversation) PopMessage() *Message {
	n := len(cv.messages)
	if n == 0 {
		return nil
	}
	if cv.system != "" && n == 1 && cv.messages[0].Role == "system" {
		return nil
	}
	last := cv.messages[n-1]
	cv.messages = cv.messages[:n-1]
	return &last
}
