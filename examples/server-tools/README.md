# server-tools

xAI's built-in server-side tools on the Responses API.

## Run

```sh
# Default, web_search
XAI_API_KEY=... go run ./examples/server-tools

# Other tools (one at a time)
go run ./examples/server-tools -tool x_search
go run ./examples/server-tools -tool code_interpreter
go run ./examples/server-tools -tool collections_search

# Custom prompt
go run ./examples/server-tools -tool web_search "What is Apple's current stock price?"
```

## What it shows

- Adding `{Type: "web_search"}` (or `x_search`, `code_interpreter`, `collections_search`) to a `responses.CreateRequest.Tools` slice, no Go handler needed; xAI executes the tool server-side
- Iterating `resp.Output` to print messages and observe the trace items the API returns alongside (reasoning steps, tool-invocation traces, etc.)
- Reading `resp.Usage.NumServerSideToolsUsed` to see how many tool invocations happened (server-side tools are priced per invocation in addition to tokens)

## When to use server-side vs. client-side tools

| Want | Use |
|---|---|
| The agent calls **your** Go code (custom logic, your data, your APIs) | `agents.Tool` + `agents.Runner` over `chat.Client` |
| The agent calls **xAI's managed** code (web, X, Python sandbox, collections) | `responses.Tool` over `responses.Client` |
| Both in one conversation | Stick with the Responses path; client-side function calling works there too via `{Type: "function", ...}` |

The `agents/` harness deliberately doesn't try to abstract the Responses API path for v1, keeping the two clients separate makes the boundary obvious. If a unified runner over `responses.Client` becomes a real need, that's a phase-2 follow-up.

## Endpoint constraint

These tool types are accepted by `/v1/responses` only. The `/v1/chat/completions` endpoint does NOT accept `web_search`, `x_search`, `code_interpreter`, or `collections_search`, passing them through `chat.CreateRequest.Tools` will return `HTTP 400`.
