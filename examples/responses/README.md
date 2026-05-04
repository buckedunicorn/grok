# responses

Multi-turn conversation using the stateful Responses API, chained via `previous_response_id`.

## Run

```sh
XAI_API_KEY=... go run ./examples/responses
```

## What it shows

- Creating a stored response with `Responses.Create` and `Store: true`
- Chaining a second turn via `PreviousResponseID`, no need to resend history
- Extracting output text from `Response.OutputText()`
- Deleting stored responses to avoid incurring storage costs
