# grok

An idiomatic, batteries-included Go client SDK for the [xAI Grok API](https://docs.x.ai/developers/rest-api-reference/inference).

```go
import grok "github.com/buckedunicorn/grok"
```

```sh
go get github.com/buckedunicorn/grok
```

## Quick start

```go
package main

import (
    "context"
    "fmt"
    "os"

    grok "github.com/buckedunicorn/grok"
    "github.com/buckedunicorn/grok/chat"
)

func main() {
    client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

    comp, err := client.Chat.Create(context.Background(), &chat.CreateRequest{
        Model: "grok-4-1-fast-non-reasoning",
        Messages: []chat.Message{
            {Role: "user", Content: "What is the speed of light?"},
        },
    })
    if err != nil {
        panic(err)
    }
    fmt.Println(comp.Choices[0].Message.Content)
}
```

## Package map

The root client (`grok.New(...)`) wires up sub-clients for every API surface. Import them as you need them.

| Package | Endpoint | Purpose |
|---|---|---|
| [`chat`](./chat) | `POST /v1/chat/completions` | OpenAI-compatible chat completions, streaming, tool use, structured output, conversation helper |
| [`responses`](./responses) | `POST /v1/responses` | The newer stateful Responses API, server-side tools (`web_search`, `x_search`, `code_interpreter`) |
| [`images`](./images) | `POST /v1/images/generations`, `/edits` | Text-to-image and image-to-image |
| [`videos`](./videos) | `POST /v1/videos/{generations,edits,extensions}` | Async video generation, with polling helper |
| [`voice`](./voice) | TTS, STT, realtime voice (HTTP and WebSocket) | Text-to-speech, speech-to-text, bidirectional voice |
| [`models`](./models) | `GET /v1/models` and friends | List and inspect available models |
| [`files`](./files) | `/v1/files` | Upload, download, list, delete files |
| [`batches`](./batches) | `/v1/batches` | Asynchronous bulk inference at reduced cost |
| [`grpc`](./grpc) | gRPC transport | Lower-overhead alternative to REST for high-throughput workloads |

### Higher-level helpers built on top of the API surface

| Package | Use it for |
|---|---|
| [`agents`](./agents) | Structured agent runtime: `Agent`, `Runner`, `Tool`, `Handoff`, `Session`, `Guardrails`, observable `Trajectory` |
| [`agents/harness`](./agents/harness) | Opinionated default agent with planning, FS, shell, and subagent tools pre-wired |
| [`agents/durable`](./agents/durable) | Checkpoint persistence (`MemoryStore`, `FileStore`) for resumable agent runs |
| [`agents/eval`](./agents/eval) | Score `RunResult` values: scorers, suites, pass@1 / latency aggregation |
| [`agents/sandbox`](./agents/sandbox) | Sandboxing implementations of the `harness.Executor` contract (Docker today) |
| [`agents/tracing`](./agents/tracing) | OpenTelemetry instrumentation for `agents.Runner` |
| [`discord`](./discord) | Discord-friendly chat sugar: tool-aware `Agent`, `Sender` interface, pre-wired media and server-side tool factories |
| [`prompt`](./prompt) | Minimal `{name}` template substitution for chat messages |
| [`runnable`](./runnable) | Generic `Runnable[In, Out]` interface and combinators (`Pipe`, `Parallel`, `Branch`, `Map`, `Retry`) |
| [`workflow`](./workflow) | Composable steps for multi-step AI pipelines |
| [`queue`](./queue) | Token-bucket rate limiter for outgoing API calls |
| [`memory`](./memory) | Conversation and agent memory backends (in-memory, file-based) |
| [`cache`](./cache) | Prompt-cache helpers for the `x-grok-conv-id` header |

## Common configuration

```go
client := grok.New(
    grok.WithAPIKey(os.Getenv("XAI_API_KEY")),
    grok.WithBaseURL("https://api.x.ai"),     // override base URL
    grok.WithHTTPClient(custom),              // bring your own *http.Client
    grok.WithConvID("conv-42"),               // x-grok-conv-id for prompt-cache locality
    grok.WithMaxRetries(3),                   // exponential backoff with jitter on 429/5xx
)
```

`grok.New` reads `XAI_API_KEY` from the environment if `WithAPIKey` is omitted. Every public method takes a `context.Context` and respects cancellation.

## Examples

The [`examples/`](./examples) directory holds runnable demonstrations of every API surface and feature combination, including:

- [`chat`](./examples/chat), [`streaming-chat`](./examples/streaming-chat), [`conversation`](./examples/conversation)
- [`responses`](./examples/responses), [`server-tools`](./examples/server-tools)
- [`images`](./examples/images), [`image-edit`](./examples/image-edit)
- [`video-from-prompt`](./examples/video-from-prompt), [`video-from-image`](./examples/video-from-image), [`video-edit`](./examples/video-edit), [`video-extend`](./examples/video-extend)
- [`tts`](./examples/tts), [`stt`](./examples/stt)
- [`tool-use`](./examples/tool-use), [`thinking`](./examples/thinking), [`decode`](./examples/decode)
- [`agents-runner`](./examples/agents-runner), [`agents-handoff`](./examples/agents-handoff), [`agents-guardrails`](./examples/agents-guardrails)
- [`harness`](./examples/harness), [`coding-agent`](./examples/coding-agent), [`sandboxed-eval`](./examples/sandboxed-eval), [`full-stack`](./examples/full-stack)
- [`durable`](./examples/durable), [`tracing`](./examples/tracing), [`eval`](./examples/eval), [`sandbox`](./examples/sandbox)
- [`discord`](./examples/discord), [`discord-bot`](./examples/discord-bot)
- [`council`](./examples/council), [`memory`](./examples/memory), [`workflow`](./examples/workflow), [`queue`](./examples/queue), [`cache`](./examples/cache), [`sugar`](./examples/sugar)

Most examples live in the root module and run with `go run ./examples/<name>`. A handful that need heavyweight third-party dependencies (notably `examples/discord-bot` with `discordgo`) are nested Go modules with their own `go.mod`; build those from inside their directory so the root module stays slim.

## Testing

The repository ships integration tests for every API surface. They read `XAI_API_KEY` from the environment and skip when absent.

```sh
go test ./...                          # unit tests only when XAI_API_KEY is unset
XAI_API_KEY=... go test ./... -v       # full integration sweep
```

Use `grok-4-1-fast-reasoning` or `grok-4-1-fast-non-reasoning` for production-accuracy tests, and `grok-3-mini-fast` for cheaper smoke tests.

## References

- [Models and pricing](https://docs.x.ai/developers/models)
- [REST API reference](https://docs.x.ai/developers/rest-api-reference/inference)
- [gRPC API reference](https://docs.x.ai/developers/grpc-api-reference)
- [Rate limits](https://docs.x.ai/developers/rate-limits)
- [Prompt caching](https://docs.x.ai/developers/advanced-api-usage/prompt-caching)
- [Server-side tools overview](https://docs.x.ai/developers/tools/overview)

