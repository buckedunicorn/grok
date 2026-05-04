# chat

`POST /v1/chat/completions`. The OpenAI-compatible chat surface, plus opinionated helpers for tool calling, streaming, structured output, and stateful conversations.

```go
import "github.com/buckedunicorn/grok/chat"
```

## When to use

`chat` is the SDK's primary entry point for text generation. Reach for it when you want a stateless completion request, a streaming response, or a tool-calling loop. For server-side tools (`web_search`, `code_interpreter`) and stateful turn chaining via `previous_response_id`, prefer the [`responses`](../responses) package.

## Surface

| API | Purpose |
|---|---|
| `Client.Create` | Single completion request |
| `Client.Stream` | Server-sent-events streaming completion |
| `Client.RunAgent` | Tool-calling loop (sends, dispatches tool calls via handlers, repeats until the model stops calling tools) |
| `Client.CreateDeferred` / `Client.GetDeferred` | Submit a request now, fetch the result later |
| `Conversation` | Stateful multi-turn helper. Tracks message history and rolls back on error |
| `Decode[T]` | Parses the first choice content as JSON into `T`. Pair with `ResponseFormat{Type: "json_schema"}` |
| `ParseList`, `ParseRegex` | Output parsers that extract list items or regex captures from a completion |
| `ThinkingContent`, `StripThinking` | Extract or remove the model's `<think>` reasoning block |

## Examples

### Streaming

```go
stream, err := client.Chat.Stream(ctx, &chat.CreateRequest{
    Model: "grok-4-1-fast-non-reasoning",
    Messages: []chat.Message{{Role: "user", Content: "Tell me a story"}},
})
if err != nil { return err }
defer stream.Close()
for {
    chunk, err := stream.Next()
    if errors.Is(err, io.EOF) { break }
    if err != nil { return err }
    fmt.Print(chunk.Choices[0].Delta.Content)
}
```

### Tool calling

`Client.RunAgent` runs the loop for you: it sends the request, dispatches every tool call to the handler in `handlers`, appends the tool results, and re-sends until the model stops requesting tools or the turn cap is reached.

```go
tools := []chat.Tool{{
    Type: "function",
    Function: chat.FunctionDef{
        Name: "lookup_weather",
        Description: "Get the current temperature in a city.",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "city": map[string]any{"type": "string"},
            },
            "required": []string{"city"},
        },
    },
}}
handlers := map[string]chat.Handler{
    "lookup_weather": func(ctx context.Context, args string) (string, error) {
        var p struct{ City string `json:"city"` }
        _ = json.Unmarshal([]byte(args), &p)
        return fmt.Sprintf(`{"temp_c":17,"city":%q}`, p.City), nil
    },
}
comp, err := client.Chat.RunAgent(ctx, &chat.CreateRequest{
    Model:    "grok-4-1-fast-non-reasoning",
    Messages: []chat.Message{{Role: "user", Content: "What's the weather in Paris?"}},
    Tools:    tools,
}, handlers, chat.WithMaxTurns(5))
```

For agent runs that need observability, multi-agent handoffs, or guardrails, prefer the higher-level [`agents.Runner`](../agents). For Discord chat bots, see [`discord.Agent`](../discord), which wraps `RunAgent` with per-channel state and tool plumbing.

### Stateful conversation

```go
conv := chat.NewConversation(client.Chat, "grok-4-1-fast-non-reasoning",
    chat.WithSystem("You are concise."))

reply1, _ := conv.Send(ctx, "Who wrote The Odyssey?")
reply2, _ := conv.Send(ctx, "When?")  // history is sent automatically
```

### Structured output

```go
type Capital struct { City string `json:"city"`; Country string `json:"country"` }

comp, _ := client.Chat.Create(ctx, &chat.CreateRequest{
    Model:    "grok-4-1-fast-non-reasoning",
    Messages: []chat.Message{{Role: "user", Content: "Name the capital of France as JSON."}},
    ResponseFormat: &chat.ResponseFormat{Type: "json_object"},
})
out, err := chat.Decode[Capital](comp)
```

## Vision

For vision-capable models, pass `[]ContentPart` for the user message instead of a string:

```go
parts := []chat.ContentPart{
    {Type: "text", Text: "What's in this picture?"},
    {Type: "image_url", ImageURL: &chat.ImageURL{URL: "https://...cat.png"}},
}
comp, _ := client.Chat.Create(ctx, &chat.CreateRequest{
    Model:    "grok-4-1-fast-non-reasoning",
    Messages: []chat.Message{{Role: "user", Content: parts}},
})
```

`Conversation.SendParts` is the multipart variant of `Send`.

## Examples directory

- [`examples/chat`](../examples/chat)
- [`examples/streaming-chat`](../examples/streaming-chat)
- [`examples/conversation`](../examples/conversation)
- [`examples/tool-use`](../examples/tool-use)
- [`examples/decode`](../examples/decode)
- [`examples/thinking`](../examples/thinking)
