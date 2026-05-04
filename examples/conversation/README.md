# conversation

Multi-turn chat with automatic history management using `chat.Conversation`.

## Run

```sh
XAI_API_KEY=... go run ./examples/conversation
```

## What it shows

- `chat.NewConversation` and `chat.WithSystem` for configuring a stateful session
- `conv.Send` appending user messages and threading the history automatically
- `conv.Messages` to inspect the full message history
- `conv.Reset` to clear history while keeping the conversation object
