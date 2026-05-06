// discord-bot is a runnable Discord persona chatbot built on top of the
// grok SDK. With the discord package's pre-wired Agent and tool
// factories, the example is mostly Discord-glue (mention parsing,
// attachment classification, reply-resolution, identity tagging), the
// chat loop, tool roster, and per-channel state are owned by the
// package.
//
// Capabilities, all driven by tools the model decides when to call:
//
//   - Conversational chat per channel, with memory.
//   - Vision: describes / reads / counts attached images.
//   - generate_image / edit_image (text-to-image, image-to-image).
//   - generate_video / extend_video, async with completion posted to
//     the channel when ready.
//   - search_web (server-side web_search + x_search).
//   - run_code (server-side code_interpreter).
//   - "!reset" command clears that channel's chat memory.
//
// Run:
//
//	cd examples/discord-bot
//	DISCORD_TOKEN=... XAI_API_KEY=... go run .
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/discord"
)

// persona is the bot's system prompt. The dynamic per-message identity
// context is prepended to each user message rather than spliced here, so
// the static prompt stays cheap to cache.
const persona = `You are Maple, a friendly conversational assistant living inside a Discord server.
You join different channels and remember the conversation in each one. Be concise (≤ 3 sentences unless the user asks for detail), warm, and a little playful.

Each user message is preceded by a short context tag in square brackets, for example:
  [alice (display: "Alice the Great") in #general (text) on My Cool Server]
Use the tag to know who you're talking to and where, but don't repeat it back unless it's directly relevant. The user did not type it.

Tools you can call:
- generate_image: text-to-image. Use when the user asks you to make / draw / paint / design a new image.
- edit_image: image-to-image. Use when the user wants a "similar" image, a variation, or any edit of an existing image. The bot uses the channel's most recent attached image automatically, you do NOT need to pass image_url.
- generate_video: text-to-video, optionally starting from an image (image-to-video). Set use_recent_image=true for image-to-video using the most recent attached image; otherwise leave it false for text-only video. Async, the bot will post the video to the channel when it's ready (1-5 minutes). Tell the user it's underway; don't keep calling the tool.
- extend_video: continue a previously-generated video. The bot uses the channel's most recent bot-generated video automatically, you do NOT need to pass video_url. Async, like generate_video.
- search_web: search the live web and X for current information. Use whenever the answer depends on recent or post-training-cutoff facts (news, prices, schedules, "what's happening with X right now"). The result already includes markdown citation links, pass them through to the user as-is rather than rephrasing them away.
- run_code: run Python in a sandbox to compute things exactly. Use for nontrivial math, date/time arithmetic, base/hex/unit conversion, regex against a sample, parsing or summarizing a small block of structured data the user pasted. The remote agent writes the code itself, describe the task. Don't reach for it for things you can already answer.

CRITICAL, never claim to have started or completed a tool action unless you actually called the corresponding tool in this turn. If a user asks for something you can't do (e.g. there's no relevant tool, or no reference image is available), say so plainly. Do not pretend.

When you generate a static image, the URL appears in the tool result and the bot also posts it to the channel automatically. You can mention what you made in plain text. When you start a video or extension, just acknowledge it and stop calling tools, the channel will receive the video on its own.`

const (
	chatModel  = "grok-4-1-fast-non-reasoning"
	maxHistory = 20 // user/assistant turn pairs kept per channel
	chunkLimit = 2000
	resetCmd   = "!reset"
)

// Bot is the long-lived process state. Most behaviour is delegated to
// discord.Agent (the chat + tool loop + per-channel history) and the
// two URLRings (recent user-attached images, recent bot-generated
// videos). Bot itself satisfies discord.Sender so the package's tools
// can post acks and media URLs back to the channel.
type Bot struct {
	grokClient *grok.Client
	ds         *discordgo.Session
	agent      *discord.Agent
	images     *discord.URLRing // recent user-attached image URLs per channel
	videos     *discord.URLRing // recent bot-generated video URLs per channel
}

func main() {
	token := mustEnv("DISCORD_TOKEN")
	apiKey := mustEnv("XAI_API_KEY")

	ds, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("discord session: %v", err)
	}
	ds.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent |
		discordgo.IntentsDirectMessages

	bot := &Bot{
		grokClient: grok.New(grok.WithAPIKey(apiKey)),
		ds:         ds,
		images:     discord.NewURLRing(8),
		videos:     discord.NewURLRing(4),
	}

	// Tool roster: media tools + server-side tools, all from the
	// discord package. Bot satisfies discord.Sender, so the tools can
	// post URLs and acks back to the channel.
	tools, handlers := bot.tools()

	bot.agent = discord.NewAgent(bot.grokClient.Chat, discord.AgentConfig{
		Model:      chatModel,
		System:     persona,
		MaxHistory: maxHistory,
		MaxTurns:   8,
		Tools:      tools,
		Handlers:   handlers,
	})

	ds.AddHandler(bot.onMessage)
	ds.AddHandler(func(s *discordgo.Session, _ *discordgo.Ready) {
		log.Printf("logged in as %s#%s (id=%s)", s.State.User.Username, s.State.User.Discriminator, s.State.User.ID)
	})

	if err := ds.Open(); err != nil {
		log.Fatalf("open gateway: %v", err)
	}
	defer func() { _ = ds.Close() }()

	log.Println("bot is running. Press Ctrl-C to stop.")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down…")
}

// tools wires the package's six tool factories. Constructed once at
// startup; the same (chat.Tool, chat.Handler) pairs are reused for
// every turn, they read per-turn channelID / userID / fallback URLs
// from context.Context, not from per-channel closures.
func (b *Bot) tools() ([]chat.Tool, map[string]chat.Handler) {
	gimg, gimgH := discord.GenerateImageTool(b.grokClient.Images, b,
		discord.WithMediaToolLogger(botlog.Printf))
	eimg, eimgH := discord.EditImageTool(b.grokClient.Images, b,
		discord.WithMediaToolLogger(botlog.Printf))
	gvid, gvidH := discord.GenerateVideoTool(b.grokClient.Videos, b, b.videos,
		discord.WithVideoToolLogger(botlog.Printf))
	evid, evidH := discord.ExtendVideoTool(b.grokClient.Videos, b, b.videos,
		discord.WithVideoToolLogger(botlog.Printf))
	search, searchH := discord.WebSearchTool(b.grokClient.Responses, b,
		discord.WithServerToolLogger(botlog.Printf))
	code, codeH := discord.RunCodeTool(b.grokClient.Responses, b,
		discord.WithServerToolLogger(botlog.Printf))

	return []chat.Tool{gimg, eimg, gvid, evid, search, code},
		map[string]chat.Handler{
			"generate_image": gimgH,
			"edit_image":     eimgH,
			"generate_video": gvidH,
			"extend_video":   evidH,
			"search_web":     searchH,
			"run_code":       codeH,
		}
}

// Send + Typing make Bot satisfy discord.Sender. Two-line adapter over
// discordgo so the package can post messages and refresh the typing
// indicator without importing discordgo itself.
func (b *Bot) Send(channelID, content string) error {
	_, err := b.ds.ChannelMessageSend(channelID, content)
	return err
}

func (b *Bot) Typing(channelID string) error { return b.ds.ChannelTyping(channelID) }

func (b *Bot) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || m.Author.ID == s.State.User.ID {
		return
	}

	// Strip bot mention up-front so reset detection works whether the
	// user said "!reset" or "@bot !reset".
	rawContent := stripBotMention(m.Content, s.State.User.ID)

	// !reset is recognized BEFORE the mention/DM gate so users can
	// clear a channel's memory with a plain "!reset" command, not just
	// by at-mentioning the bot.
	if strings.EqualFold(rawContent, resetCmd) {
		b.agent.Reset(m.ChannelID)
		b.images.Reset(m.ChannelID)
		b.videos.Reset(m.ChannelID)
		botlog.Printf("[ch=%s usr=%s] !reset", short(m.ChannelID), m.Author.Username)
		_, _ = s.ChannelMessageSend(m.ChannelID, "🧹 Memory cleared for this channel.")
		return
	}

	// Otherwise: respond in DMs always; in guild channels only when
	// @-mentioned.
	isDM := m.GuildID == ""
	isMention := mentionsBot(m, s.State.User.ID)
	if !isDM && !isMention {
		return
	}

	imgs := imageAttachments(m.Attachments)
	if rawContent == "" && len(imgs) == 0 {
		return
	}

	// Track user-attached images so future turns have a stored
	// fallback even after the message scrolls out of the trimmed
	// history window. (LLMs reliably mangle long Discord-CDN URLs
	// when copying them out of conversation history.)
	for _, a := range imgs {
		b.images.Add(m.ChannelID, a.URL)
	}

	fallbackImage, fallbackVideo := b.resolveFallbacks(s, m)

	botlog.Printf("[ch=%s usr=%s] msg %q (images=%d, reply=%t, fb_img=%t, fb_vid=%t)",
		short(m.ChannelID), m.Author.Username, preview(rawContent, 80), len(imgs),
		m.MessageReference != nil, fallbackImage != "", fallbackVideo != "")

	// Build the user message: text + (optional) image_url parts.
	textPart := buildContextTag(s, m)
	if rawContent != "" {
		textPart += "\n" + rawContent
	}
	var userContent any = textPart
	if len(imgs) > 0 {
		parts := make([]chat.ContentPart, 0, 1+len(imgs))
		parts = append(parts, chat.ContentPart{Type: "text", Text: textPart})
		for _, img := range imgs {
			parts = append(parts, chat.ContentPart{
				Type:     "image_url",
				ImageURL: &chat.ImageURL{URL: img.URL, Detail: "auto"},
			})
		}
		userContent = parts
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Keep "is typing…" alive across the full turn (single ping lasts
	// ~10s; tool-using turns can take 30-60s).
	defer discord.KeepTyping(b, m.ChannelID, 7*time.Second)()

	// Per-turn context: who's talking + best fallback URLs the package
	// tools should use when the model omits image_url / video_url.
	ctx = discord.WithMessageContext(ctx, discord.MessageContext{
		UserID:        m.Author.ID,
		FallbackImage: fallbackImage,
		FallbackVideo: fallbackVideo,
	})

	answer, err := b.handleViaAgent(ctx, m.ChannelID, userContent)
	if err != nil {
		botlog.Printf("[ch=%s] grok error: %v", short(m.ChannelID), err)
		_, _ = s.ChannelMessageSend(m.ChannelID, "Sorry, I had trouble responding. Try again?")
		return
	}
	if answer == "" {
		return
	}
	botlog.Printf("[ch=%s] reply: %q", short(m.ChannelID), preview(answer, 80))
	for _, chunk := range discord.Chunks(answer, chunkLimit) {
		if _, err := s.ChannelMessageSend(m.ChannelID, chunk); err != nil {
			botlog.Printf("[ch=%s] send error: %v", short(m.ChannelID), err)
			return
		}
	}
}

// handleViaAgent dispatches a turn to the agent. Routes by content
// shape because Agent has separate methods for text-only vs.
// vision-capable input.
func (b *Bot) handleViaAgent(ctx context.Context, channelID string, content any) (string, error) {
	if parts, ok := content.([]chat.ContentPart); ok {
		return b.agent.HandleParts(ctx, channelID, parts)
	}
	text, _ := content.(string)
	return b.agent.Handle(ctx, channelID, text)
}

// resolveFallbacks computes the best fallback image and video URLs for
// the package's tools to use when the model omits image_url /
// video_url. Priority for each:
//
//	current message  →  replied-to message  →  stored most-recent in channel
//
// Stored most-recent comes from the URLRings, Bot.images is
// populated in onMessage from user attachments; b.videos is populated
// by the package's GenerateVideoTool poll routine on success.
func (b *Bot) resolveFallbacks(s *discordgo.Session, m *discordgo.MessageCreate) (image, video string) {
	image = firstImageIn(m.Message)
	video = firstVideoIn(m.Message)
	if image == "" || video == "" {
		if ref := referencedMessage(s, m); ref != nil {
			if image == "" {
				image = firstImageIn(ref)
			}
			if video == "" {
				video = firstVideoIn(ref)
			}
		}
	}
	if image == "" {
		image = b.images.Recent(m.ChannelID)
	}
	if video == "" {
		video = b.videos.Recent(m.ChannelID)
	}
	return image, video
}

// ---------------------------------------------------------------------------
// Discord-message classification: everything below is glue between
// discordgo's types and the package's URL/string helpers. None of it
// could move into the discord package without dragging in the
// discordgo dependency.
// ---------------------------------------------------------------------------

// supportedImageExts is the set of attachment file extensions xAI's
// vision pipeline accepts.
var supportedImageExts = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {},
}

var supportedImageContentTypes = map[string]struct{}{
	"image/png": {}, "image/jpeg": {}, "image/gif": {}, "image/webp": {},
}

// supportedVideoExts / supportedVideoContentPrefix are the file types
// we consider videos when scanning a message's attachments. xAI's
// Extend endpoint accepts MP4 reliably; WEBM and MOV usually work too.
var supportedVideoExts = map[string]struct{}{
	".mp4": {}, ".webm": {}, ".mov": {},
}

const supportedVideoContentPrefix = "video/"

func isImageAttachment(a *discordgo.MessageAttachment) bool {
	if a == nil || a.URL == "" {
		return false
	}
	if _, ok := supportedImageContentTypes[strings.ToLower(a.ContentType)]; ok {
		return true
	}
	_, ok := supportedImageExts[strings.ToLower(filepath.Ext(a.Filename))]
	return ok
}

func isVideoAttachment(a *discordgo.MessageAttachment) bool {
	if a == nil || a.URL == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(a.ContentType), supportedVideoContentPrefix) {
		return true
	}
	_, ok := supportedVideoExts[strings.ToLower(filepath.Ext(a.Filename))]
	return ok
}

func imageAttachments(atts []*discordgo.MessageAttachment) []*discordgo.MessageAttachment {
	var out []*discordgo.MessageAttachment
	for _, a := range atts {
		if isImageAttachment(a) {
			out = append(out, a)
		}
	}
	return out
}

// firstImageIn returns the first image URL discoverable in msg  -
// attachments first (most reliable), then image-shaped URLs in
// content. "" if none found.
func firstImageIn(msg *discordgo.Message) string {
	if msg == nil {
		return ""
	}
	for _, a := range msg.Attachments {
		if isImageAttachment(a) {
			return a.URL
		}
	}
	for _, u := range discord.ExtractURLs(msg.Content) {
		if discord.LooksLikeImageURL(u) {
			return u
		}
	}
	return ""
}

func firstVideoIn(msg *discordgo.Message) string {
	if msg == nil {
		return ""
	}
	for _, a := range msg.Attachments {
		if isVideoAttachment(a) {
			return a.URL
		}
	}
	for _, u := range discord.ExtractURLs(msg.Content) {
		if discord.LooksLikeVideoURL(u) {
			return u
		}
	}
	return ""
}

// referencedMessage returns the message m is replying to, or nil when
// m isn't a reply or the lookup fails. Hits the State cache first to
// keep the hot path off the network.
func referencedMessage(s *discordgo.Session, m *discordgo.MessageCreate) *discordgo.Message {
	if m.MessageReference == nil || m.MessageReference.MessageID == "" {
		return nil
	}
	chID := m.MessageReference.ChannelID
	if chID == "" {
		chID = m.ChannelID
	}
	msgID := m.MessageReference.MessageID
	if msg, err := s.State.Message(chID, msgID); err == nil && msg != nil {
		return msg
	}
	msg, err := s.ChannelMessage(chID, msgID)
	if err != nil {
		return nil
	}
	return msg
}

func mentionsBot(m *discordgo.MessageCreate, botID string) bool {
	for _, u := range m.Mentions {
		if u != nil && u.ID == botID {
			return true
		}
	}
	return false
}

func stripBotMention(msg, botID string) string {
	for _, token := range []string{"<@" + botID + ">", "<@!" + botID + ">"} {
		msg = strings.ReplaceAll(msg, token, "")
	}
	return strings.TrimSpace(msg)
}

// buildContextTag prepends every user message with a short bracketed
// identity string so the model knows who and where it's talking
// without having to ingest the static system prompt every turn.
func buildContextTag(s *discordgo.Session, m *discordgo.MessageCreate) string {
	var sb strings.Builder
	sb.WriteString("[")

	user := m.Author
	sb.WriteString(user.Username)
	if displayName := serverDisplayName(m); displayName != "" && !strings.EqualFold(displayName, user.Username) {
		fmt.Fprintf(&sb, ` (display: %q)`, displayName)
	}
	fmt.Fprintf(&sb, " id=%s", user.ID)

	if m.GuildID == "" {
		sb.WriteString(" in DM")
	} else {
		ch, err := s.State.Channel(m.ChannelID)
		if err != nil {
			ch, err = s.Channel(m.ChannelID)
		}
		if err == nil && ch != nil {
			fmt.Fprintf(&sb, " in #%s (%s)", ch.Name, channelTypeName(ch.Type))
		}
		guild, err := s.State.Guild(m.GuildID)
		if err != nil {
			guild, err = s.Guild(m.GuildID)
		}
		if err == nil && guild != nil {
			fmt.Fprintf(&sb, " on %s", guild.Name)
		}
	}
	sb.WriteString("]")
	return sb.String()
}

func serverDisplayName(m *discordgo.MessageCreate) string {
	if m.Member != nil && m.Member.Nick != "" {
		return m.Member.Nick
	}
	if m.Author != nil && m.Author.GlobalName != "" {
		return m.Author.GlobalName
	}
	return ""
}

func channelTypeName(t discordgo.ChannelType) string {
	switch t {
	case discordgo.ChannelTypeGuildText:
		return "text"
	case discordgo.ChannelTypeDM:
		return "DM"
	case discordgo.ChannelTypeGuildVoice:
		return "voice"
	case discordgo.ChannelTypeGroupDM:
		return "group-DM"
	case discordgo.ChannelTypeGuildCategory:
		return "category"
	case discordgo.ChannelTypeGuildNews:
		return "announcement"
	case discordgo.ChannelTypeGuildStore:
		return "store"
	case discordgo.ChannelTypeGuildNewsThread:
		return "announcement-thread"
	case discordgo.ChannelTypeGuildPublicThread:
		return "public-thread"
	case discordgo.ChannelTypeGuildPrivateThread:
		return "private-thread"
	case discordgo.ChannelTypeGuildStageVoice:
		return "stage"
	case discordgo.ChannelTypeGuildForum:
		return "forum"
	default:
		return "unknown"
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatal(errors.New(key + " is not set"))
	}
	return v
}

// botlog is the bot's activity logger. Routes through the standard log
// package today; centralized so it's easy to swap in slog or tee to a
// file later.
var botlog = log.Default()

// short returns the first 8 chars of an id for compact log lines.
func short(id string) string {
	const n = 8
	if len(id) <= n {
		return id
	}
	return id[:n]
}

// preview returns the first max characters of s with newlines escaped,
// so a noisy multi-line message turns into a single readable log line.
func preview(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
