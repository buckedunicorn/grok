# Changelog

All notable changes to this project are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

While the major version is `0`, the API may break between minor releases. Pin to an exact tag (`go get github.com/buckedunicorn/grok@vX.Y.Z`) in production.

## [Unreleased]

Nothing yet. Pending work lands under one of: Added, Changed, Deprecated, Removed, Fixed, Security.

## [0.1.2] - 2026-05-07

### Security

- Upgraded `golang.org/x/net` from v0.49.0 to v0.53.0, resolving GO-2026-4918 (infinite loop in HTTP/2 transport with malformed `SETTINGS_MAX_FRAME_SIZE`).
- Raised minimum Go version to 1.26.3, resolving GO-2026-4971 (panic in `net.Dialer` and `net.LookupPort` on NUL byte input on Windows).

## [0.1.1] - 2026-05-07

### Added

- `LICENSE` (MIT) — omitted from v0.1.0; required for the module to be legally usable.
- `Example*` functions in `example_test.go` files for the root package, `chat`, `responses`, `images`, `videos`, and `agents`. These appear as expandable code examples on pkg.go.dev.

### Changed

- Expanded package-level doc comments for `chat`, `images`, `videos`, `files`, and `models` to describe key types and usage patterns.
- Root package doc now lists every sub-client with godoc cross-links.
- `agents` and `chat` package docs use `[pkg.Type]` link syntax throughout.
- Annotated git tag message format updated in `RELEASING.md` from bare version string to brief subject line (e.g. `grok vX.Y.Z`).

## [0.1.0] - 2026-05-06

Initial public release.

### Added

#### Root client

- `grok.New(opts...)` returning `*grok.Client` with sub-clients for every API surface.
- Functional options: `WithAPIKey`, `WithBaseURL`, `WithInsecureBaseURL`, `WithHTTPClient`, `WithConvID`, `WithMaxRetries`, `WithLogger`, `WithConcurrency`.
- Reads `XAI_API_KEY` from the environment when no `WithAPIKey` is supplied.
- `*grok.APIError` with `StatusCode`, `Code`, `Message`, `Raw` fields; matches via `errors.As`.

#### REST API surface

- [`chat`](./chat) — `POST /v1/chat/completions`: `Create`, `Stream`, `CreateDeferred` / `GetDeferred`, `RunAgent` tool-use loop, structured output via `Decode[T]`, output parsers (`ParseList`, `ParseRegex`, `StripThinking`, `ThinkingContent`), stateful `Conversation` helper.
- [`responses`](./responses) — `POST /v1/responses`: `Create`, `Stream`, `Get`, `Delete`. Chains turns server-side via `previous_response_id`; supports server-side tools (`web_search`, `x_search`, `code_interpreter`).
- [`images`](./images) — `Generate` / `Edit` with single and multi-reference inputs.
- [`videos`](./videos) — `Generate` / `Edit` / `Extend` (async with `request_id`) plus `GetResult` and `Wait` polling helper.
- [`voice`](./voice) — HTTP `TextToSpeech`, `Transcribe`, `ListVoices`, `GetVoice`, `CreateEphemeralToken`; WebSocket sub-packages [`voice/realtime`](./voice/realtime), [`voice/ttsstream`](./voice/ttsstream), [`voice/sttstream`](./voice/sttstream).
- [`models`](./models) — list and inspect across `models`, `language-models`, `image-generation-models`, `video-generation-models`.
- [`files`](./files) — `Upload` / `UploadPath` / `Download` / `List` / `Get` / `Delete` with `iter.Seq2` pagination via `All`.
- [`batches`](./batches) — full CRUD plus `AddRequests`, `GetResults`, `Cancel`, `Wait`, and `iter.Seq2` iterators (`All`, `AllRequests`, `AllResults`).
- [`grpc`](./grpc) — `Client` skeleton with TLS dial and bearer-token interceptor; vendored protobuf bindings from [`xai-org/xai-proto`](https://github.com/xai-org/xai-proto) under `grpc/gen/`.

#### Agent runtime

- [`agents`](./agents) — `Runner` over `chat.Client` with structured `Trajectory`, `RunHooks`, `Middleware`, `Guardrails`, multi-agent `Handoff`, and `Session` (in-memory and `SessionFromConversation`).
- [`agents/harness`](./agents/harness) — opinionated default agent: planning (`write_todos`, `list_todos`), in-memory FS (`read_file`, `write_file`, `edit_file`, `list`, `glob`, `grep`), pluggable shell `Executor` (off by default), subagent (`task`) spawner. `MemoryFS` and `LocalFS` (rooted, traversal-safe) implementations of `FS`.
- [`agents/durable`](./agents/durable) — `MemoryStore` and `FileStore` (atomic JSON-per-runID via temp+rename) plus `Resume` helper. Wires through `Runner.Checkpointer` and `RunOptions.Resume`.
- [`agents/eval`](./agents/eval) — `Scorer` interface with `Contains`, `Equal`, `Regex`, `JSONField`, `AllOf`, `AnyOf`, `Not`, `LLMJudge`. `Suite.Run` aggregates pass@1, mean tokens, mean turns, p50/p95 latency; `Report.Print` and `WriteCSV` emit summaries.
- [`agents/sandbox`](./agents/sandbox) — `DockerExecutor` shells out to `docker run --rm` per call with hardening defaults (`network=none`, `cap-drop=ALL`, `no-new-privileges`, read-only root with `/tmp` tmpfs, `pids-limit=256`, user `65534:65534`). Stdlib-only.
- [`agents/tracing`](./agents/tracing) — OpenTelemetry adapter: `Hooks(tracer)` returns `RunHooks` emitting per-turn / per-tool / per-handoff spans; `Middleware(tracer)` wraps each `Run` in a parent `agent.run` span.

#### Discord and helpers

- [`discord`](./discord) — `Sender` abstraction (Discord-library agnostic), tool-aware `Agent` wrapping `chat.RunAgent`, pre-wired media tools (image generation, image edit, video generation, video extend), server-side tools (web search, code interpreter), helpers (`KeepTyping`, `Mention`, `Chunks`, `FetchAsDataURI`, `URLRing`).
- [`prompt`](./prompt) — minimal `{name}` template substitution for `chat.Message`.
- [`runnable`](./runnable) — generic `Runnable[In, Out]` and combinators (`Pipe`, `Pipe3`, `Pipe4`, `Parallel`, `Branch`, `Map`, `ParallelMap`, `Retry`).
- [`workflow`](./workflow) — composable string-step pipelines.
- [`queue`](./queue) — token-bucket rate limiter for outbound API calls, with optional ahead-cap.
- [`memory`](./memory) — `Memory` interface with `InMemory` and `File` (atomic JSON) implementations.
- [`cache`](./cache) — sugar around the `x-grok-conv-id` header for prompt-prefix caching.
- [`council`](./council) — multi-agent `Roundtable` and `Synthesize` patterns over `chat.Client`.

#### Examples and tooling

- 41 runnable examples under [`examples/`](./examples) covering every API surface and helper.
- `examples/discord-bot` standalone Go submodule integrating `discordgo`.
- CI workflow (`gofmt`, `go vet`, `go build`, `go test -race -shuffle=on`, `govulncheck`, `gosec`, plus a matrix submodule build).
- `SECURITY.md` disclosure policy.
- `docs/` developer guides: [getting-started](./docs/getting-started.md), [configuration](./docs/configuration.md), [streaming-and-tools](./docs/streaming-and-tools.md), [production](./docs/production.md).

[Unreleased]: https://github.com/buckedunicorn/grok/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/buckedunicorn/grok/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/buckedunicorn/grok/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/buckedunicorn/grok/releases/tag/v0.1.0
