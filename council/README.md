# council

Multi-agent roundtable and synthesis patterns built on top of `chat.Client`. Ask N agents the same question concurrently; either return all answers, or have a moderator agent synthesize them into one.

```go
import "github.com/buckedunicorn/grok/council"
```

## When to use

Reach for `council` when you want N independent answers to the same prompt. Common shapes:

- Self-consistency: ask the same model N times at non-zero temperature; pick the modal answer or have a moderator merge them. Reduces variance on reasoning tasks.
- Mixture of experts: route the same question to N differently-configured agents (different models, different system prompts) and synthesize.
- A/B comparison: run the same prompt against an "old" and a "new" system prompt and surface disagreements.

For multi-agent flows where the agents talk to *each other* rather than vote, reach for [`agents`](../agents) and `Handoff` instead.

## Surface

| Type / function | Role |
|---|---|
| `Participant` | One council seat: name, `*chat.Client`, model, optional system prompt |
| `Roundtable(ctx, question, participants)` | Runs every participant concurrently, returns one `Response` per seat |
| `Synthesize(ctx, question, participants, moderator)` | Runs Roundtable, then asks `moderator` to merge the answers |
| `Response` | One seat's answer: name, output, usage, optional error |

Per-participant API errors land in `Response.Err`; the rest of the seats still return. Context cancellation propagates to every in-flight request.

## Example

```go
client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

participants := []council.Participant{
    {Name: "fast",      Client: client.Chat, Model: "grok-3-mini-fast"},
    {Name: "reasoning", Client: client.Chat, Model: "grok-4-1-fast-reasoning"},
    {Name: "balanced",  Client: client.Chat, Model: "grok-4-1-fast-non-reasoning"},
}
moderator := council.Participant{Name: "judge", Client: client.Chat, Model: "grok-4-1-fast-reasoning"}

answer, err := council.Synthesize(ctx, "Is the Riemann Hypothesis proven? Explain in one sentence.", participants, moderator)
if err != nil { return err }
fmt.Println(answer)
```

## Examples directory

- [`examples/council`](../examples/council)
