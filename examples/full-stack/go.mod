module github.com/buckedunicorn/grok/examples/full-stack

go 1.26.2

// Use the in-repo grok and grok/agents/tracing modules rather than published
// versions. If you copy this example into a standalone repo, drop the
// replace directives and bump the requires to real tags.
replace (
	github.com/buckedunicorn/grok => ../..
	github.com/buckedunicorn/grok/agents/tracing => ../../agents/tracing
)

require (
	github.com/buckedunicorn/grok v0.0.0
	github.com/buckedunicorn/grok/agents/tracing v0.0.0
	go.opentelemetry.io/otel v1.43.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.43.0
	go.opentelemetry.io/otel/sdk v1.43.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/grpc v1.80.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
