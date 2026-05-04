# council

Multi-agent roundtable and synthesis with `council.Roundtable` and `council.Synthesize`.

## Run

```sh
XAI_API_KEY=... go run ./examples/council
```

## What it shows

- Defining multiple `council.Participant` structs with distinct personas and system prompts
- `council.Roundtable` dispatches the same question to all participants in parallel and returns results in input order
- `council.Synthesize` runs a moderator participant over all responses to produce a single balanced recommendation
- Participants can use different models, useful for mixing fast and capable models in the same council
