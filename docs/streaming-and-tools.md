# Streaming and tool use

The two non-trivial flows on top of `chat.Client.Create`. Read this if you are building anything beyond one-shot prompts.

## Streaming

Set `Stream: true` and call `Stream` (not `Create`). You receive a `*chat.Stream` whose `Next` method returns one `*chat.Chunk` at a time, returning `io.EOF` at the end.

### Pull-driven

```go
s, err := client.Chat.Stream(ctx, &chat.CreateRequest{
    Model:    "grok-4-1-fast-non-reasoning",
    Messages: []chat.Message{{Role: "user", Content: "Write a haiku about Go."}},
    Stream:   true,
})
if err != nil { return err }
defer s.Close()

for {
    chunk, err := s.Next()
    if errors.Is(err, io.EOF) { break }
    if err != nil { return err }

    if len(chunk.Choices) > 0 {
        fmt.Print(chunk.Choices[0].Delta.Content)
    }
}
```

### Collect to a string

For "give me the whole thing as one string but stream it under the hood" use `Collect`:

```go
s, _ := client.Chat.Stream(ctx, req)
text, err := s.Collect()
```

`Collect` only returns the first choice's content. For `N > 1` or to read final-chunk usage, loop manually.

### Usage on the final chunk

Set `Stream: true` and pass `stream_options.include_usage = true` (the SDK exposes this via the request struct on the Responses API; chat completions surface it through the streaming response itself). The final `*chat.Chunk` has a non-nil `Usage`.

### Aborting mid-stream

Cancel the context. `Next` returns `ctx.Err()` on the next read; `Close` releases the underlying connection.

```go
ctx, cancel := context.WithCancel(parent)
defer cancel()
// in another goroutine, when the user clicks "stop":
cancel()
```

The SSE reader is buffered, so up to one chunk after cancellation may still be delivered before `Next` reports the context error. Drain or discard that chunk; the connection is gone after `Close`.

## Tool use

The model decides to call one of your registered tools by emitting a `tool_calls` payload. You execute the tool, append the result, and call again. There are two ways to run that loop.

### `chat.Client.RunAgent` (the lightweight option)

A single procedural function that loops for you. Register tool definitions and handlers; pass an initial request; receive the final `*chat.Completion` once the loop terminates.

```go
tools := []chat.Tool{
    {
        Type: "function",
        Function: chat.FunctionDef{
            Name:        "get_weather",
            Description: "Look up the current temperature in a city.",
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "city": map[string]any{"type": "string"},
                },
                "required": []string{"city"},
            },
        },
    },
}

handlers := map[string]chat.Handler{
    "get_weather": func(ctx context.Context, args string) (string, error) {
        var p struct{ City string }
        if err := json.Unmarshal([]byte(args), &p); err != nil { return "", err }
        return fmt.Sprintf("%s: 17C", p.City), nil
    },
}

req := &chat.CreateRequest{
    Model:    "grok-4-1-fast-non-reasoning",
    Tools:    tools,
    Messages: []chat.Message{{Role: "user", Content: "What is the weather in Tokyo?"}},
}

final, err := client.Chat.RunAgent(ctx, req, handlers, chat.WithMaxTurns(8))
if err != nil { return err }
fmt.Println(final.Choices[0].Message.Content)
```

`RunAgent` never mutates the caller's `req.Messages` slice. The loop terminates when `finish_reason != "tool_calls"` or when `WithMaxTurns` is exceeded; in the latter case the error wraps `chat.ErrMaxTurnsExceeded`.

Use `RunAgent` when you want a one-shot answer and do not care about per-turn observability.

### `agents.Runner` (the structured option)

The same loop, but produces a `RunResult` with a turn-by-turn `Trajectory` you can inspect, persist, score, or trace. Reach for `agents.Runner` when:

- You want to log or replay every tool call.
- You want input/output guardrails.
- You want multi-agent handoff.
- You want to checkpoint runs and resume them ([`agents/durable`](../agents/durable)).

```go
agent := &agents.Agent{
    Name:         "weather-bot",
    Instructions: "Answer succinctly.",
    Model:        "grok-4-1-fast-non-reasoning",
    Tools:        weatherTools,  // []agents.Tool, structurally similar to chat.Tool
}

runner := &agents.Runner{Client: client.Chat, MaxTurns: 8}
res, err := runner.Run(ctx, agent, agents.RunOptions{Input: "Weather in Tokyo?"})
if err != nil { return err }

fmt.Println(res.Output)
fmt.Println("turns:", len(res.Trajectory.Turns))
```

### Streaming tool use

Stream + tool calls is supported by the API but more involved on the client side: tool-call arguments stream in pieces and you must accumulate them before dispatching. Most callers stream non-tool turns and switch to non-streaming mode for the turn that actually calls a tool. The `agents.Runner` path does not yet stream intermediate tokens (the underlying loop calls `Create`, not `Stream`).

## See also

- [`chat`](../chat) for the full chat-completions reference.
- [`agents`](../agents) for the structured runtime.
- [`docs/production.md`](./production.md) for sandboxing tool execution and tracing tool-call latency.
