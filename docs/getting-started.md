# Getting started

This guide walks from `go get` to a first working request and the failure modes you will hit along the way. It assumes Go 1.26 or newer.

## Install

```sh
go get github.com/buckedunicorn/grok
```

## Authenticate

Every request needs an xAI API key. Two ways to supply it, in order of precedence:

1. Pass `grok.WithAPIKey(key)` to `grok.New`. This wins if both are set.
2. Set the `XAI_API_KEY` environment variable. `grok.New` reads it automatically when no `WithAPIKey` option is supplied.

The constructor returns an error from your first API call (not from `New`) if neither is present, so unit tests that never call out can construct a client with no key.

## A first request

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
        Model: "grok-3-mini-fast",
        Messages: []chat.Message{
            {Role: "user", Content: "What is the speed of light?"},
        },
    })
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
    fmt.Println(comp.Choices[0].Message.Content)
}
```

`grok.New` returns a `*Client` whose fields are typed sub-clients (`Chat`, `Responses`, `Images`, `Videos`, `Voice`, `Models`, `Files`, `Batches`, `GRPC`). Every method takes a `context.Context` as its first argument and respects cancellation.

## Picking a model

Use `grok-3-mini-fast` for cheap smoke tests during development. For production accuracy, prefer one of:

- `grok-4-1-fast-reasoning` — adds hidden chain-of-thought (`reasoning_tokens` show up in usage)
- `grok-4-1-fast-non-reasoning` — same family, no hidden reasoning, lower latency

The model catalog is at <https://docs.x.ai/developers/models>. From code, `client.Models.List(ctx)` enumerates the models your key has access to.

## When things go wrong

Errors returned from any sub-client method are one of three shapes:

| Cause | Type to match | Fields |
|---|---|---|
| The API returned a non-2xx response | `*grok.APIError` | `StatusCode`, `Code`, `Message`, `Raw` |
| The HTTP transport failed (timeout, connection reset, DNS) | a wrapped `net/http` error | unwrap with `errors.Is(err, context.DeadlineExceeded)` etc. |
| You passed an invalid argument before the request went out | a plain `errors.New` from this SDK | `err.Error()` describes the field |

Match `*grok.APIError` to read the API's structured response:

```go
import "errors"

var apiErr *grok.APIError
if errors.As(err, &apiErr) {
    if apiErr.StatusCode == 429 {
        // rate-limited; back off
    }
}
```

Common failure causes on a first run:

- **`401 unauthorized`** — `XAI_API_KEY` was empty or wrong. Check `echo $XAI_API_KEY` and confirm it has no surrounding whitespace.
- **`404 not found` on a model name** — the model id was misspelled or your account does not have access. `client.Models.List(ctx)` lists what you can use.
- **`context deadline exceeded`** — the default `http.Client` has no timeout, so this only fires when you set `context.WithTimeout` yourself. The SDK respects the context, so use it.

## Where to read next

- [`docs/configuration.md`](./configuration.md) — every `With*` option, every header, every env var.
- [`docs/streaming-and-tools.md`](./streaming-and-tools.md) — patterns for SSE streaming and tool-use loops.
- [`docs/production.md`](./production.md) — retries, rate limiting, prompt caching, sandboxing, observability.
- The per-package READMEs under [`chat/`](../chat), [`responses/`](../responses), [`images/`](../images), etc.
- Runnable examples under [`examples/`](../examples).
