# responses

`POST /v1/responses` and friends. The newer, stateful Responses API. Chains turns server-side via `previous_response_id`, supports server-side tools (`web_search`, `x_search`, `code_interpreter`, `collections_search`).

```go
import "github.com/buckedunicorn/grok/responses"
```

## When to use

Use `responses` when you want:

- Server-side tools that execute on xAI's infrastructure (you provide a type-only entry; xAI handles the search/code/etc. and returns results inline).
- Stateful turn chaining without re-sending full history (`PreviousResponseID`).
- xAI's recommended path for new agentic features. The Live Search hooks on `/v1/chat/completions` are deprecated in favor of `web_search` here.

For OpenAI-compatible chat with client-side tool dispatch, use [`chat`](../chat) instead.

## Surface

| API | Purpose |
|---|---|
| `Client.Create` | Single request, returns a `*Response` with one or more `Output` items |
| `Client.Stream` | Streaming variant. Yields `StreamEvent` values |
| `Client.Get` | Fetch a stored response by ID |
| `Client.Delete` | Delete a stored response |

## Example: server-side web search

```go
resp, err := client.Responses.Create(ctx, &responses.CreateRequest{
    Model: "grok-4-1-fast-reasoning",
    Input: "What was today's top science story?",
    Tools: []responses.Tool{
        {Type: "web_search"},
        {Type: "x_search"},
    },
})
if err != nil { return err }
fmt.Println(resp.OutputText())
fmt.Println("server tools used:", resp.Usage.NumServerSideToolsUsed)
fmt.Println("sources:", resp.Usage.NumSourcesUsed)
```

`OutputText()` walks `Response.Output` and concatenates the text content of message items. Inline citations of the form `[[N]](url)` appear in that text by default; set `include=["no_inline_citations"]` to suppress them.

## Example: chaining turns

```go
first, _ := client.Responses.Create(ctx, &responses.CreateRequest{
    Model: "grok-4-1-fast-non-reasoning",
    Input: "Translate 'hello' to French.",
    Store: ptr(true),
})
second, _ := client.Responses.Create(ctx, &responses.CreateRequest{
    Model:              "grok-4-1-fast-non-reasoning",
    Input:              "Now Spanish.",
    PreviousResponseID: first.ID,
})
```

Set `Store: ptr(true)` to opt into 30-day server-side persistence so future requests can chain via `PreviousResponseID`.

## Examples directory

- [`examples/responses`](../examples/responses)
- [`examples/server-tools`](../examples/server-tools)
