# files

File upload, list, and delete using `client.Files`.

## Run

```sh
XAI_API_KEY=... go run ./examples/files
```

## What it shows

- Uploading an `io.Reader` with `Files.Upload`
- Listing files with `Files.List` and checking `FileList.HasMore()`
- Deleting a file with `Files.Delete`
