# discord-bot

A runnable Discord persona chatbot. Per-channel chat memory + dynamic identity context + vision + image generation + image-to-video, all on top of `grok`.

This example is a **separate Go module** (its own `go.mod`) so the `github.com/bwmarrin/discordgo` dependency stays out of the core grok module. The root `go build ./...` does not pull it in.

## What it does

- **Persona system prompt**, `Maple`, a concise / friendly bot, configurable in `main.go` via the `persona` constant.
- **Per-channel chat memory**, one history slice per channel, locked behind a per-channel mutex, trimmed past `maxHistory` user/assistant pairs.
- **Identity + location context per message**, the model sees a one-line bracketed tag with the user's username + server display name + snowflake id + channel name + Discord channel type + guild name, built from `discordgo`'s State cache (with API fallback).
- **Vision**, image attachments (`.png` / `.jpg` / `.jpeg` / `.gif` / `.webp`) are forwarded to grok as `image_url` parts. The model can describe / read / count / compare attached images. Non-image attachments are silently dropped.
- **Image generation** via the `generate_image` tool, text-to-image. The bot posts the result URL straight to the channel; Discord auto-embeds.
- **Image edits / "similar image"** via the `edit_image` tool, image-to-image with a reference URL the model finds in conversation history.
- **Video generation** via the `generate_video` tool, text-to-video, optionally with a starting image (image-to-video). Async: the tool acks immediately, a background goroutine polls xAI for completion (1-5 minutes typical) and posts the video URL when ready.
- **Web + X search** via the `search_web` tool, kicks off a one-shot Responses API call with xAI's server-side `web_search` and `x_search` tools enabled, returns the synthesized answer with markdown citations. Use for news / prices / "what's happening right now" / anything past the model's training cutoff.
- **Sandboxed Python** via the `run_code` tool, wraps xAI's server-side `code_interpreter`. The model describes a task; the remote agent writes and runs Python in a sandbox; the answer comes back as text. Useful for math, date arithmetic, regex, parsing user-pasted data, etc.
- **`!reset`** clears that channel's chat memory. Recognized standalone (no `@bot` prefix needed).
- **Long-reply chunking**, replies are split with `discord.Chunks(text, 2000)` and sent as separate messages.
- **DM mode**, direct messages always get a reply. In guild channels the bot only responds when @-mentioned (or to a bare `!reset`).

## Setup

### 1. Create the Discord application + bot

1. Open <https://discord.com/developers/applications> and click **New Application**. Name it whatever you like.
2. Sidebar → **Bot**. Click **Add Bot** if prompted. Copy the **Token**, that's your `DISCORD_TOKEN`.
3. On the same Bot page, scroll to **Privileged Gateway Intents** and enable **Message Content Intent**. (Without this the bot receives empty strings for every message.)

### 2. Invite the bot to a server

1. Sidebar → **OAuth2 → URL Generator**.
2. **Scopes**: check `bot`.
3. **Bot permissions**: at minimum `Send Messages`, `Read Message History`, `View Channels`. Add `Read Messages in Threads` if you want thread support.
4. Copy the generated URL, open it in a browser, pick a server, click **Authorize**.

### 3. Run it

```sh
cd examples/discord-bot
DISCORD_TOKEN=<your bot token> XAI_API_KEY=<your xAI key> go run .
```

Then in your server, mention the bot:

```
@Maple hi! who am I?
```

## Vision + generation in action

```
@Maple [attach a photo of a cat]
       describe this in detail
        ↓
The bot describes the photo.

@Maple please generate a similar image
        ↓
edit_image tool fires with the original image's URL.
The bot posts the new image URL; Discord auto-embeds it.

@Maple turn that cat into a 5-second video
        ↓
generate_video tool fires with the image URL.
"On it, I'll post the video when it's ready (~1-5 min)."
[a couple minutes later]
<@user> https://...video.mp4
```

## How the context tag is built

```
[username (display: "Server Nickname or Global Name") id=<snowflake> in #<channel> (<type>) on <Guild>]
```

Each piece is best-effort, if the State cache miss falls through to the API and that also fails, the missing piece is just dropped. Display name is omitted when it equals the username.

The tag is plain text inside square brackets; the persona prompt tells the model not to repeat it back.

## Customizing

| Knob | Where |
|---|---|
| Persona / instructions | `persona` const in `main.go` |
| Chat model | `chatModel` const (default `grok-4-1-fast-non-reasoning`) |
| Memory window | `maxHistory` const (user/assistant pairs per channel) |
| Trigger rule (mention vs. always) | `onMessage` in `main.go`, current rule: DM always, guild on @-mention |
| Reset command syntax | `resetCmd` const |
| Chunk size | `chunkLimit` const (Discord max is 2000) |
| Tool definitions / handlers | `tools.go` |
| Video poll interval / timeout | `videoPollInterval` / `videoPollTimeout` consts in `tools.go` |

## Architecture

The bot uses `chat.Client.RunAgent` rather than `discord.Router` because it needs tool-calling. Per-channel state (history + mutex) is managed inline:

```
onMessage
  ├── strip @-mention, detect !reset, gate by mention/DM
  ├── build user message (string or []ContentPart with image URLs)
  └── runTurn(channelID, userContent)
        ├── lock channel mutex
        ├── append user msg to channel history
        ├── chat.RunAgent(system + history, tools, handlers)
        │     ├── generate_image  → images.Generate, post URL to channel
        │     ├── edit_image      → images.Edit,     post URL to channel
        │     └── generate_video  → videos.Generate, fork goroutine to poll + post
        ├── append assistant final answer to history
        └── trim history to last 2*maxHistory msgs (user-aligned)
```

History stored: user message + final assistant text only. Intermediate `tool_calls` / `tool` messages are not persisted across turns, the assistant's textual reply mentions any URLs it generated, so future turns can refer back to them via the assistant's words alone. Avoids orphan-tool-call errors when trimming and keeps history shape simple.

## Limitations / not in scope

- **No persistence across restarts.** Chat memory is in-process; restarting the bot clears every channel.
- **No slash commands.** Application-command registration would need extra setup on bot startup.
- **Generated URLs may expire.** Both image and video URLs from xAI are short-lived signed URLs; Discord embeds work while they're live but won't be permanent. For durability, download the bytes server-side and re-upload as a Discord file attachment (out of scope here).
- **No rate limiting.** discordgo handles Discord's gateway rate limits; for grok-side throttling, wrap calls in `queue.Queue.Submit`.
- **Trust-the-model image-URL extraction.** The `edit_image` and `generate_video` tools require the model to copy the image URL out of recent conversation history. Reliable for current-turn references; less reliable many turns later when the URL has scrolled out of the trimmed window.
