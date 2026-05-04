# grpc

Skeleton for the gRPC transport. Lower-overhead alternative to REST for high-throughput inference workloads.

```go
import xgrpc "github.com/buckedunicorn/grok/grpc"
```

## Status

The package ships:

- A `Client` skeleton with TLS dial to `api.x.ai:443`.
- An auth interceptor that injects `Authorization: Bearer <key>` on outgoing metadata.
- Generated protobuf types under `grpc/gen/...` from the upstream [xai-org/xai-proto](https://github.com/xai-org/xai-proto) repository.

To wire in a service client, generate the bindings with `buf generate` against `xai-org/xai-proto` and embed the generated client into `grpc.Client`. The skeleton handles the dial and auth wiring.

## Generating bindings

```sh
git clone https://github.com/xai-org/xai-proto
cd xai-proto
buf generate --template buf.gen.yaml
```

Copy or vendor the generated Go packages into `grpc/gen/` (the existing tree shows the layout).

## Auth

`AuthContext(ctx)` appends the bearer token to outgoing metadata. The interceptor calls it on every RPC, so call sites do not need to thread the key manually.

## Reference

- [xAI gRPC API reference](https://docs.x.ai/developers/grpc-api-reference)
- [xai-org/xai-proto](https://github.com/xai-org/xai-proto)

## Examples directory

- [`examples/grpc`](../examples/grpc)
