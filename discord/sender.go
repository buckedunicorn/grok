package discord

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Sender is the minimal Discord write surface the helpers in this
// package need. Wrap your Discord client library (discordgo, disgord,
// raw HTTP, …) into a 5-line adapter and pass it where the helpers
// require a Sender.
//
//	type dgSender struct{ s *discordgo.Session }
//	func (a dgSender) Send(channelID, content string) error {
//	 _, err := a.s.ChannelMessageSend(channelID, content)
//	 return err
//	}
//	func (a dgSender) Typing(channelID string) error {
//	 return a.s.ChannelTyping(channelID)
//	}
//
// Keeping this interface narrow is deliberate, it's what lets the
// discord package stay free of any Discord-library dependency.
type Sender interface {
	// Send posts content to channelID. Implementations should return a
	// non-nil error if the write failed; callers typically log and
	// continue rather than abort.
	Send(channelID, content string) error
	// Typing pings Discord's "is typing…" indicator for channelID. The
	// indicator times out at ~10s server-side; KeepTyping refreshes it.
	Typing(channelID string) error
}

// KeepTyping starts a background goroutine that re-pings sender.Typing
// for channelID every interval until the returned stop function is
// called. Discord's typing indicator times out around ten seconds;
// without a refresher a long agent turn (e.g. 30–60s while a server-
// side search runs) leaves the user staring at silence. Use:
//
//	stop := discord.KeepTyping(sender, channelID, 7*time.Second)
//	defer stop()
//	answer, err := agent.Handle(ctx, channelID, msg)
//
// stop is idempotent and safe to call from any goroutine.
//
// If interval is ≤ 0, defaults to 7 seconds. The first ping fires
// immediately so the indicator appears without waiting an interval.
func KeepTyping(sender Sender, channelID string, interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = 7 * time.Second
	}
	stopCh := make(chan struct{})
	var once sync.Once
	go func() {
		// First ping is treated identically to subsequent pings: a
		// failure (channel deleted, perms revoked, gateway flapped)
		// stops the goroutine. Previously the first error was silently
		// dropped, leaving us looping for `interval` before noticing
		//.
		if err := sender.Typing(channelID); err != nil {
			return
		}
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-t.C:
				if err := sender.Typing(channelID); err != nil {
					return
				}
			}
		}
	}()
	return func() { once.Do(func() { close(stopCh) }) }
}

// Mention returns Discord's @-user mention syntax with a trailing space
// so it composes naturally with following content. Returns empty string
// when userID is empty (silently no-ops in templated send patterns).
func Mention(userID string) string {
	if userID == "" {
		return ""
	}
	return "<@" + userID + "> "
}

// LooksLikeURL reports whether s starts with http(s):// and contains a
// hostname with a dot. Conservative, it filters obvious model
// hallucinations (placeholders, truncations) without doing full URL
// parsing.
func LooksLikeURL(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return false
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	host := rest
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		host = rest[:i]
	}
	return strings.Contains(host, ".") && host != "."
}

// LooksLikeImageURL reports whether s is a URL whose path ends in a
// common image extension (.png .jpg .jpeg .gif .webp), case-insensitive.
// Use it as a cheap classifier on user-pasted links before fetching or
// passing to a vision model. Query strings are ignored.
func LooksLikeImageURL(s string) bool {
	if !LooksLikeURL(s) {
		return false
	}
	return hasFoldExtension(stripQuery(s), imageExts)
}

// LooksLikeVideoURL reports whether s is a URL whose path ends in a
// common video extension (.mp4 .webm .mov), or whose host is a
// known xAI video CDN.
func LooksLikeVideoURL(s string) bool {
	if !LooksLikeURL(s) {
		return false
	}
	if hasFoldExtension(stripQuery(s), videoExts) {
		return true
	}
	return containsFold(s, "vidgen.x.ai")
}

// hasFoldExtension reports whether path ends in any of exts, ignoring
// case. Avoids the strings.ToLower allocation that would lowercase the
// entire URL just to check a 4-byte suffix.
func hasFoldExtension(path string, exts []string) bool {
	for _, ext := range exts {
		if len(path) >= len(ext) && strings.EqualFold(path[len(path)-len(ext):], ext) {
			return true
		}
	}
	return false
}

// containsFold reports whether s contains substr, ignoring case, in
// linear time without lowercasing the whole input.
func containsFold(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if strings.EqualFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

// ExtractURLs returns whitespace-delimited tokens in text that look
// like URLs. Strips Markdown angle brackets and common surrounding
// punctuation ("see <https://x.example/y>!" → ["https://x.example/y"]).
// Order is preserved; duplicates are NOT deduped, use a set if you
// want unique URLs.
func ExtractURLs(text string) []string {
	if text == "" {
		return nil
	}
	var out []string
	for tok := range strings.FieldsSeq(text) {
		tok = strings.Trim(tok, "<>()[]{}\"'.,!?;:")
		if LooksLikeURL(tok) {
			out = append(out, tok)
		}
	}
	return out
}

// stripQuery removes any "?…" query string from a URL.
func stripQuery(u string) string {
	before, _, _ := strings.Cut(u, "?")
	return before
}

var (
	imageExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}
	videoExts = []string{".mp4", ".webm", ".mov"}
)

// channelCtxKey scopes Agent.Handle's per-call channel ID inside the
// context it passes to RunAgent (and thus to tool handlers). Tool
// handlers retrieve it via ChannelFromContext to know which Discord
// channel to address (post acks, look up per-channel state, etc.).
type channelCtxKey struct{}

// WithChannelContext returns ctx augmented with channelID. Agent.Handle
// applies this automatically before invoking chat.RunAgent; you only
// need it directly when you're calling a discord-package tool handler
// outside an Agent.
func WithChannelContext(ctx context.Context, channelID string) context.Context {
	return context.WithValue(ctx, channelCtxKey{}, channelID)
}

// ChannelFromContext returns the channel ID previously stored on ctx by
// WithChannelContext, or "" if none. Tool handlers call this to scope
// their behavior (where to post acks, which channel's state to read).
// Returning "" means "no channel context", handlers should degrade
// gracefully (skip ack posts, etc.) rather than failing.
func ChannelFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(channelCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// MessageContext is per-turn Discord-message metadata that the
// pre-wired media tools (GenerateImageTool, EditImageTool,
// GenerateVideoTool, ExtendVideoTool) consult on every call.
//
// Convention: tool handlers built outside this package
// that want to honour the same per-turn metadata should call
// MessageContextFrom(ctx) at the top of their Handler. Forgetting to
// call it silently drops the user ID and fallback URLs; the tool will
// still run but cannot post a media URL with @-mention or fall back
// to the Discord-resolved reference URL.
//
// - UserID is the author of the user message that started this
// turn. Used by media tools to @-mention the user when they post a
// generated image / video URL back to the channel, so the user
// gets a notification.
//
// - FallbackImage is the best image URL to use as a reference when
// the model omits one on edit_image / generate_video. The example
// bot computes it as "image attached to current msg → image
// attached to replied-to msg → most-recent user-attached image
// stored per channel". The package never inspects Discord
// attachments itself.
//
// - FallbackVideo is the same idea for extend_video. The package's
// ExtendVideoTool also consults its optional URLRing of
// recent bot-generated videos; FallbackVideo wins when set.
//
// All fields are optional. Empty strings degrade gracefully (a missing
// FallbackImage just means edit_image will return an error if the
// model omits image_url too).
type MessageContext struct {
	UserID        string
	FallbackImage string
	FallbackVideo string
}

type messageCtxKey struct{}

// WithMessageContext stores mc on ctx. Call this once per turn before
// Agent.Handle, e.g.:
//
//	ctx = discord.WithMessageContext(ctx, discord.MessageContext{
//	 UserID: m.Author.ID,
//	 FallbackImage: resolvedImage,
//	 FallbackVideo: resolvedVideo,
//	})
//	answer, err := agent.Handle(ctx, channelID, content)
func WithMessageContext(ctx context.Context, mc MessageContext) context.Context {
	return context.WithValue(ctx, messageCtxKey{}, mc)
}

// MessageContextFrom returns the MessageContext previously stored on
// ctx, or its zero value if none. Tool handlers call this to find the
// per-turn UserID and fallback URLs.
func MessageContextFrom(ctx context.Context) MessageContext {
	if v, ok := ctx.Value(messageCtxKey{}).(MessageContext); ok {
		return v
	}
	return MessageContext{}
}
