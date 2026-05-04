# files

`/v1/files` endpoints. Upload, download, list, delete files used by the inference, batch, and collections APIs.

```go
import "github.com/buckedunicorn/grok/files"
```

## Surface

| API | Purpose |
|---|---|
| `Client.Upload` | Upload from an `io.Reader` |
| `Client.UploadPath` | Convenience: open and upload a file by path |
| `Client.Download` | Download a file's content as a stream |
| `Client.Get` | Fetch metadata for one file |
| `Client.List` | Paginated list. `FileList.NextToken` and `HasMore()` for manual paging |
| `Client.All` | Iterator (`iter.Seq2[*File, error]`) over every file. Break to stop pagination |
| `Client.Delete` | Delete a file |

## Example

```go
f, err := client.Files.UploadPath(ctx, "training-data.jsonl")
if err != nil { return err }
fmt.Println("uploaded", f.ID, f.Filename)

for file, err := range client.Files.All(ctx, nil) {
    if err != nil { return err }
    fmt.Println(file.ID, file.Bytes, file.Filename)
}
```

## Examples directory

- [`examples/files`](../examples/files)
