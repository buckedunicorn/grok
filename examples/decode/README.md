# decode

Structured JSON output with `chat.Decode[T]`.

## Run

```sh
XAI_API_KEY=... go run ./examples/decode
```

## What it shows

- Passing a `ResponseFormat` with `type: "json_schema"` to constrain the model's output
- `chat.Decode[Recipe]` generic helper that asserts the content string and calls `json.Unmarshal` in one step, eliminating boilerplate
- Defining a Go struct that mirrors the JSON schema for type-safe access to the response
