# memory

Conversation and agent memory backends.

```go
import "github.com/buckedunicorn/grok/memory"
```

## When to use

`memory` lets you persist facts and notes that an agent can read across turns or restarts. Two backends ship with the SDK:

- `InMemory`: ephemeral in-process storage. Useful for tests and for any agent whose lifetime matches the host process.
- `File`: JSON-backed persistence on disk. Survives restarts.

Both satisfy the `Store` interface, so callers can swap implementations without touching call sites.

## Surface

| Type / function | Purpose |
|---|---|
| `Store` | Backend interface: `Save`, `Load`, `Delete`, `List` |
| `InMemory` | In-process implementation |
| `File` | File-backed implementation |
| `AsSystemFragment(store, key)` | Renders the stored value as a system-prompt fragment ready to splice into a chat request |

## Example

```go
store := memory.NewInMemory()
_ = store.Save(ctx, "user-prefs", "Prefers metric units, replies in Spanish.")

fragment, _ := memory.AsSystemFragment(store, "user-prefs")
msgs := append([]chat.Message{{Role: "system", Content: fragment}}, history...)
client.Chat.Create(ctx, &chat.CreateRequest{Model: "...", Messages: msgs})
```

## Examples directory

- [`examples/memory`](../examples/memory)
