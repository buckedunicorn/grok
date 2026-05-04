# models

`/v1/models` and friends. List and inspect available models.

```go
import "github.com/buckedunicorn/grok/models"
```

## Surface

| API | Endpoint | Purpose |
|---|---|---|
| `Client.List` | `GET /v1/models` | Compact catalog (id + a few fields) |
| `Client.Get` | `GET /v1/models/{id}` | Full record for one model |
| `Client.ListLanguage` | `GET /v1/language-models` | Pricing details for chat models |
| `Client.GetLanguage` | `GET /v1/language-models/{id}` | Full record for one language model |
| `Client.ListImageGeneration` | `GET /v1/image-generation-models` | Image-generation models |
| `Client.ListVideoGeneration` | `GET /v1/video-generation-models` | Video-generation models |

## Example

```go
list, err := client.Models.ListLanguage(ctx)
if err != nil { return err }
for _, m := range list {
    fmt.Printf("%s ctx=%d input=$%.2f/Mtok\n", m.ID, m.ContextWindow, m.InputCostPerMillion)
}
```

## Examples directory

- [`examples/models`](../examples/models)
