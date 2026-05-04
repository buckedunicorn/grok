# thinking

Extracting and stripping chain-of-thought reasoning with `chat.ThinkingContent` and `chat.StripThinking`.

## Run

```sh
XAI_API_KEY=... go run ./examples/thinking
```

## What it shows

- Setting `ReasoningEffort: "low"` to control reasoning depth on a model that exposes the knob
- `chat.ThinkingContent(comp)` returns the model's chain-of-thought from either the dedicated `reasoning_content` field (current xAI reasoning models) or inline `<think>...</think>` tags in the response content (legacy / simple models)
- `chat.StripThinking(comp)` returns a shallow copy of the completion with both pathways scrubbed, leaving a clean final answer
- Reading `comp.Usage.CompletionTokensDetails.ReasoningTokens` to see how many tokens were consumed by reasoning even when the trace itself is hidden

## Model choice

This example uses `grok-3-mini-fast` because it accepts `reasoning_effort`. The `grok-4-1-fast-reasoning` family reasons *intrinsically* and rejects `reasoning_effort` (`HTTP 400 Model … does not support parameter reasoningEffort`). Both families return reasoning via `reasoning_content`, so `ThinkingContent` works against either, just drop the `ReasoningEffort` line if switching to a grok-4 reasoning model.
