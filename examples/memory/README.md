# memory

Persistent context injection with `memory.InMemory` and `memory.File`.

## Run

```sh
XAI_API_KEY=... go run ./examples/memory
```

## What it shows

- `memory.InMemory` for a lightweight in-process key/value store ordered by insertion time
- `memory.NewFile(path)` for a JSON-backed store that survives process restarts
- `store.Set`, `store.Get`, `store.All` for reading and writing entries
- `memory.AsSystemFragment(store)` formats all entries as a system-prompt block that the model can reference
- Reopening a `File` store and verifying that previously written entries survive
