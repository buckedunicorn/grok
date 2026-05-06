# discord

Discord-friendly chat sugar. Stdlib-only and Discord-library agnostic. Wires `chat.Client` into per-channel patterns that any Discord client (discordgo, disgord, raw HTTP) can plug into via a small `Sender` adapter.

```go
import "github.com/buckedunicorn/grok/discord"
```

## Pick a runtime

| Runtime | Use it for |
|---|---|
| `Router` | Tool-free per-channel chat. Lightest option, backed by `chat.Conversation`. |
| `Agent` | Tool-aware per-channel chat backed by `chat.RunAgent`. Threads `channelID`, `userID`, and per-turn fallback URLs onto `context.Context` so pre-wired tools can scope themselves without per-channel closures. |

Both serialize calls on the same channel ID and run different channels in parallel.

## The Sender interface

```go
type Sender interface {
    Send(channelID, content string) error
    Typing(channelID string) error
}
```

This is the only thing the package needs from a Discord client. A typical adapter is two lines:

```go
type dgSender struct{ s *discordgo.Session }
func (a dgSender) Send(channelID, content string) error {
    _, err := a.s.ChannelMessageSend(channelID, content)
    return err
}
func (a dgSender) Typing(channelID string) error {
    return a.s.ChannelTyping(channelID)
}
```

Pass a `Sender` to the helpers that post messages back to Discord.

## Pre-wired tools

Each factory returns a `(chat.Tool, chat.Handler)` pair you plug into `AgentConfig.Tools` / `Handlers`:

| Factory | What it does |
|---|---|
| `WebSearchTool` | Server-side `web_search` + `x_search` via the Responses API. Posts a "Searching" ack to the channel before the subcall. |
| `RunCodeTool` | Server-side `code_interpreter` via the Responses API. Posts a "Running Python" ack. |
| `GenerateImageTool` | `Images.Generate`. Posts the resulting URL with the user's @-mention. |
| `EditImageTool` | `Images.Edit`. Resolves the reference image from `MessageContext.FallbackImage`. |
| `GenerateVideoTool` | `Videos.Generate` plus async poll. Inlines reference images as base64 data URIs to bypass cross-service fetcher quirks (notably xAI video vs Discord CDN). On completion, posts the URL and remembers it in the shared `URLRing`. |
| `ExtendVideoTool` | `Videos.Extend` plus async poll. Resolves the reference video from `MessageContext.FallbackVideo`, falling back to the shared `URLRing` of recent bot-generated videos. |

## Helpers

| Helper | Purpose |
|---|---|
| `KeepTyping(sender, channelID, interval)` | Refreshes Discord's typing indicator until the returned `stop` is called. Single ping lasts ~10s server-side; long agent turns (30-60s with a server-side search) need a refresher. |
| `Mention(userID)` | Formats `<@USERID> ` (with trailing space). |
| `Chunks(text, max)` | Splits a reply across Discord's 2000-char per-message cap. |
| `LooksLikeURL`, `LooksLikeImageURL`, `LooksLikeVideoURL` | Conservative classifiers for user-pasted or model-emitted links. |
| `ExtractURLs(text)` | Strips Markdown angle brackets and surrounding punctuation, returns URL-shaped tokens. |
| `FetchAsDataURI(ctx, url, opts...)` | Downloads bytes and returns a `data:<mime>;base64,...` URI. Useful when one xAI service can't fetch from another host. |
| `URLRing` | Per-channel ring buffer of URLs. Shared between `GenerateVideoTool` and `ExtendVideoTool` to track recent bot-generated videos. |

## Context plumbing

`Agent.Handle` populates two `context.Context` values automatically:

- `WithChannelContext(ctx, channelID)` lets tool handlers know which channel they're acting in (for posting acks, scoping per-channel state).
- `WithMessageContext(ctx, MessageContext{UserID, FallbackImage, FallbackVideo})` (set by the caller before `Handle`) carries per-turn message metadata. Handlers retrieve it via `MessageContextFrom(ctx)`.

The fallback URLs are a recurring pattern: when the model omits `image_url` / `video_url`, the tools use the URL the caller resolved from "current message attachment, replied-to message attachment, stored most-recent in channel" instead of asking the model to copy a long Discord-CDN URL out of conversation history (which it sometimes mangles).

## Example

```go
import (
    "github.com/buckedunicorn/grok"
    "github.com/buckedunicorn/grok/chat"
    "github.com/buckedunicorn/grok/discord"
)

client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

videoMem := discord.NewURLRing(4)

gimg, gimgH := discord.GenerateImageTool(client.Images, sender)
gvid, gvidH := discord.GenerateVideoTool(client.Videos, sender, videoMem)
evid, evidH := discord.ExtendVideoTool(client.Videos, sender, videoMem)
search, searchH := discord.WebSearchTool(client.Responses, sender)

agent := discord.NewAgent(client.Chat, discord.AgentConfig{
    Model:      "grok-4-1-fast-non-reasoning",
    System:     "You are a friendly assistant.",
    MaxHistory: 20,
    Tools:      []chat.Tool{gimg, gvid, evid, search},
    Handlers: map[string]chat.Handler{
        "generate_image": gimgH,
        "generate_video": gvidH,
        "extend_video":   evidH,
        "search_web":     searchH,
    },
})

// In your Discord message handler:
ctx = discord.WithMessageContext(ctx, discord.MessageContext{
    UserID:        m.Author.ID,
    FallbackImage: resolvedImageURL,
    FallbackVideo: resolvedVideoURL,
})
defer discord.KeepTyping(sender, channelID, 7*time.Second)()
reply, err := agent.Handle(ctx, channelID, userMessage)
```

## Trust boundaries

Discord chatbots sit at a particularly hostile trust boundary: any user in the server can post a message that the model will see, and prompt injection can steer the model to call tools with attacker-supplied arguments. The package is hardened accordingly:

- `FetchAsDataURI` validates URLs before any network activity. By default only `https://` is accepted; the resolved host must be a public IP. Loopback, RFC 1918, link-local, multicast, and unspecified addresses are rejected. Redirects are re-validated. The dial-time check re-resolves to defeat DNS rebinding. Use `WithFetchAllowHTTP`, `WithFetchAllowPrivateAddresses`, and `WithFetchAllowList(hosts...)` to relax the guard explicitly when you trust a specific host or run against local fixtures.
- `URLRing` is per-channel and bounded; a busy attacker cannot use it to grow process memory unboundedly.
- The pre-wired media tools sanity-check model-supplied URLs via `LooksLikeURL` before passing them to xAI; obvious model hallucinations (placeholders, truncated strings) are rejected.
- The pre-wired server-side tools post a visible "Searching" or "Running Python" ack to the channel, which doubles as an auditable activity log via the optional logger.

What the package does not do for you:

- The package does not impose per-user rate limiting. Wrap calls in [`queue.Queue.Submit`](../queue) (use `WithMaxQueueAhead` to refuse new work past a horizon) when you need it.
- The package does not sanitise message content before it reaches the model. Apply your own input validation (length caps, content scoping, abuse classifier) in front of `Agent.Handle`.
- The package does not redact secrets from `Agent.Messages`. Use `agents.RunHooks.RedactToolArgs` / `RedactToolResult` if you wire the tools through `agents.Runner` instead.

For the SDK's full security posture see [`SECURITY.md`](../SECURITY.md).

## Examples directory

- [`examples/discord`](../examples/discord) (Router, no tools)
- [`examples/discord-bot`](../examples/discord-bot) (Agent with the full tool roster)
