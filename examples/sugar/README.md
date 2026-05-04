# sugar

Composes the optional langchain-style helpers, `prompt`, `runnable`, and `chat.Decode`, into a single typed pipeline.

## Run

```sh
XAI_API_KEY=... go run ./examples/sugar
```

## What it shows

- `prompt.ChatTemplate` renders System/User templates with `{var}` substitution into `[]chat.Message`
- `runnable.Func[In, Out]` adapts plain functions into the universal `Runnable` interface
- `runnable.Pipe3` composes three stages, render, call, parse, into one typed end-to-end pipeline
- `chat.Decode[Recipe]` is the JSON output parser, fitting cleanly as the final stage
- The composed pipeline takes `map[string]any` (template variables) and returns a typed `Recipe`, no manual JSON wrangling at the call site

When to reach for this: pipelines with 3+ stages where each stage's input/output types are stable enough to commit to. For one-off completions, `client.Chat.Create` directly is still the simplest path.
