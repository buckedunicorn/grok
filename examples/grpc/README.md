# grpc

Chat completion (blocking and streaming) via the xAI gRPC API.

## Run

```sh
XAI_API_KEY=... go run ./examples/grpc
```

## What it shows

- Creating a gRPC client with `client.NewGRPC()`
- Attaching auth with `gc.AuthContext(ctx)`, pass this context to every RPC
- Calling `gc.Chat.GetCompletion` for a blocking response
- Calling `gc.Chat.GetCompletionChunk` and streaming with `stream.Recv()`
- Building proto `Message` and `Content` values with `MessageRole` enums

The generated service clients are in `grpc/gen/xai/api/v1`. Services not in `grpc.Client` fields (e.g. `management_api` billing) are accessible via `gc.Conn()` to construct clients manually.
