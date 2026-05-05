# Configuration

Every dial in one place: `grok.New` options, request-level overrides, environment variables, and the headers the SDK sends.

## `grok.New` options

Pass these to the root constructor. Order does not matter; later wins on conflict.

| Option | Default | Effect |
|---|---|---|
| `WithAPIKey(key)` | reads `XAI_API_KEY` env var | The bearer token sent on `Authorization`. Empty key = no auth header (every request 401s). |
| `WithBaseURL(url)` | `https://api.x.ai` | The host every sub-client points at. Useful for staging endpoints, fakes, and HTTP-level recording proxies. Must be `https://` unless paired with `WithInsecureBaseURL`. |
| `WithInsecureBaseURL()` | off | Allows `WithBaseURL` to accept `http://`. Only intended for local recording proxies and tests; never use in production. |
| `WithHTTPClient(hc)` | `&http.Client{}` (no timeout, default transport) | Replace the underlying client. Use this to plug in connection pooling, custom dialers, or instrumented transports. The SDK never sets `Client.Timeout`; control deadlines via `context.WithTimeout` instead. |
| `WithConvID(id)` | empty (no header) | Sends `x-grok-conv-id: id` on every request. The API uses it to share a prompt-prefix cache across turns of the same logical conversation. See `docs/production.md` for the cache-hit story. |
| `WithMaxRetries(n)` | 0 (no retries) | Enable exponential-backoff retry on 429 and 5xx responses and on transient network errors. Each retry honours `Retry-After` when set. Set to 3 in production. |
| `WithLogger(l)` | nil | Attach a `*slog.Logger` that receives one structured event per request and per retry. Levels: `Debug` for request/response, `Warn` for retried failures, `Error` for terminal failures. |
| `WithConcurrency(n)` | unbounded | Limits in-flight HTTP requests to `n` across all sub-clients of this `*Client`. Pair with `WithMaxRetries` to keep retry storms from saturating downstream. |

Methods on the constructed `*Client` that produce a scoped copy:

| Method | Effect |
|---|---|
| `client.WithConvID(id)` | Returns a shallow copy of the client whose sub-clients send `x-grok-conv-id: id`. The original client is unchanged. |

## Per-request fields

Some settings live on the request struct, not the client, because they vary per call:

| Where | Field | Purpose |
|---|---|---|
| `chat.CreateRequest`, `responses.CreateRequest` | `Stream bool` | Switches the call to SSE; use `Client.Stream` instead of `Create` when set. |
| `chat.CreateRequest` | `ReasoningEffort string` | `"low"` or `"high"`. Reasoning models only. |
| `responses.CreateRequest` | `Reasoning *Reasoning{Effort: ...}` | Same idea, structured. Accepts `"low"`, `"medium"`, `"high"`. |
| All requests | `User string` | Optional caller-supplied user identifier. The API uses it for abuse-rate accounting; recommended in any multi-tenant frontend. |
| `chat.CreateRequest`, `responses.CreateRequest` | `SearchParameters *SearchParams` | Live web/X search injection. |
| `chat.CreateRequest`, `responses.CreateRequest` | `ResponseFormat / Text.Format` | JSON-only or JSON-Schema-constrained output. |

## Environment variables

Read on construction or at request time. None are required if you supply the equivalent option.

| Variable | Read by | Effect |
|---|---|---|
| `XAI_API_KEY` | `grok.New` | Used as the API key when no `WithAPIKey` is supplied. |

The SDK does not read any other environment variable on its own. The discord-bot example reads `DISCORD_BOT_TOKEN`; the agents/sandbox docker executor inherits whatever the host docker daemon needs (`DOCKER_HOST`, etc.); but those are not the SDK's contract.

## Headers the SDK sends

Every request:

| Header | Value | Source |
|---|---|---|
| `Authorization` | `Bearer <key>` | `WithAPIKey` or `XAI_API_KEY` |
| `Content-Type` | `application/json` for JSON bodies; `multipart/form-data; boundary=...` for uploads | Request kind |
| `User-Agent` | `grok-go/<version>` | Built-in |
| `x-grok-conv-id` | the configured conv-id | `WithConvID` (omitted if not set) |

## Streaming, polling, and timeouts

The SDK never imposes its own timeout. Always wrap calls in `context.WithTimeout` (or `WithDeadline`) at the level of the operation you are doing:

| Operation | Realistic timeout |
|---|---|
| `chat.Client.Create` (non-reasoning) | 30s |
| `chat.Client.Create` (reasoning) | 2-5 min |
| `chat.Client.Stream` | per-chunk read; bound the whole stream loosely (e.g. 5 min) |
| `videos.Client.Wait` | 5-15 min depending on model |
| `voice/realtime.Session` | open-ended; bound by your application logic, not by ctx |

The retry layer (`WithMaxRetries`) honours the request's context: if the context is already done, no retries happen.

## See also

- [`docs/getting-started.md`](./getting-started.md) for the first-call walkthrough.
- [`docs/streaming-and-tools.md`](./streaming-and-tools.md) for streaming details.
- [`docs/production.md`](./production.md) for the operational settings (retries, caching, observability).
