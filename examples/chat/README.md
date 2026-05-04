# chat

Single non-streaming chat completion using `client.Chat.Create`.

## Run

```sh
XAI_API_KEY=... go run ./examples/chat
```

## What it shows

- Creating a `grok.Client` with an API key
- Sending a `chat.CreateRequest` with system and user messages
- Reading the response text and token usage from `Completion.Choices`
