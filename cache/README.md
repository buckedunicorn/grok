# cache

Helpers for xAI prompt caching. xAI automatically caches prompt prefixes; cache hits are most likely when the same `x-grok-conv-id` header is sent across turns in the same conversation.

```go
import "github.com/buckedunicorn/grok/cache"
```

## How prompt caching works

xAI quantizes and stores recent KV state keyed by `x-grok-conv-id`. Subsequent requests with the same header that share the same prompt prefix get billed for cached tokens at a steep discount and complete faster.

The root client exposes the conv-id directly:

```go
client := grok.New(
    grok.WithAPIKey(os.Getenv("XAI_API_KEY")),
    grok.WithConvID("session-42"),
)
```

`client.WithConvID(id)` returns a shallow copy with a different conv-id, so you can scope a single client to per-session conversations without re-creating the whole transport.

## Surface

| API | Purpose |
|---|---|
| `NewID()` | Generate a fresh conversation id (random, URL-safe) |
| `IDFromContext`, `WithID` | Thread a conv-id via `context.Context` for libraries that want to forward it without parameter plumbing |

The actual header injection happens inside the transport when `Client.WithConvID` is used; this package is for callers that want id minting and context plumbing helpers.

## Example

```go
convID := cache.NewID()
scoped := client.WithConvID(convID)

// Every request through `scoped` carries x-grok-conv-id: convID.
comp, _ := scoped.Chat.Create(ctx, &chat.CreateRequest{...})
fmt.Println("cached prompt tokens:", comp.Usage.PromptTokensDetails.CachedTokens)
```

## Examples directory

- [`examples/cache`](../examples/cache)
