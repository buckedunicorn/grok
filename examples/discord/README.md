# discord

Discord-pattern adapter using `discord.Router` and `discord.Chunks`.

## Run

```sh
XAI_API_KEY=... go run ./examples/discord
```

## What it shows

- `discord.NewRouter` manages one `chat.Conversation` per channel ID, keeping history isolated between channels
- `router.Handle(ctx, channelID, message)` returns the bot reply as a plain string, wire this into whichever Discord library you use (`discordgo`, `disgo`, `arikawa`, etc.)
- `discord.Chunks(text, 2000)` splits a reply at word boundaries to fit Discord's 2000-character message limit; send each chunk as a separate message
- `router.Reset(channelID)` clears a channel's conversation history (e.g. on a `/reset` slash command)
- No third-party dependencies, the package is a zero-import-dependency adapter over `chat.Conversation`
