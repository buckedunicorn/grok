# cache

Prompt-cache hit-rate tracking with `cache.Manager`.

## Run

```sh
XAI_API_KEY=... go run ./examples/cache
```

## What it shows

- Creating a `cache.Manager` to generate and own a stable `x-grok-conv-id` value
- Passing `grok.WithConvID(mgr.ConvID())` so every request in the session shares the same cache prefix
- Calling `mgr.Record(comp.Usage)` after each completion to accumulate statistics
- Reading `mgr.Stats().HitRate()` to verify the cache is warming across turns
