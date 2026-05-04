# prompt

Minimal `{name}` template substitution for chat messages. Stdlib-only.

```go
import "github.com/buckedunicorn/grok/prompt"
```

## Why this exists

The Go standard library's `text/template` is powerful and well-tested. Use it when you need conditionals, loops, or pipelines. `prompt.Template` is for the 90% case: substitute named variables into a string, no control flow.

The shorter syntax (`{name}` instead of `{{ .Name }}`) reads better when the result is a chat message that a model will see.

## Surface

| Type / function | Purpose |
|---|---|
| `Template` | A parsed template. Use `New(text)` to build, `Render(vars)` to substitute |
| `ChatTemplate` | A list of system/user templates. Renders to `[]chat.Message` |

## Example

```go
tmpl := prompt.New("Translate '{phrase}' to {language}.")
out, err := tmpl.Render(map[string]any{
    "phrase":   "good morning",
    "language": "Japanese",
})
// out == "Translate 'good morning' to Japanese."
```

For multi-message templates:

```go
ct := prompt.NewChatTemplate(
    prompt.NewSystem("You are a translator."),
    prompt.NewUser("Translate '{phrase}' to {language}."),
)
msgs, _ := ct.Render(map[string]any{"phrase": "hi", "language": "French"})
client.Chat.Create(ctx, &chat.CreateRequest{Model: "...", Messages: msgs})
```

## Examples directory

- [`examples/sugar`](../examples/sugar) (composes `prompt` + `chat` + `runnable`)
