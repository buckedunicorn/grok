# streaming-chat

Token-by-token streaming chat completion using `client.Chat.Stream`.

## Run

```sh
XAI_API_KEY=... go run ./examples/streaming-chat
```

## What it shows

- Opening an SSE stream with `Chat.Stream`
- Reading `Chunk` values with `stream.Next()` until `io.EOF`
- Printing delta content as it arrives
- Properly closing the stream with `defer stream.Close()`
