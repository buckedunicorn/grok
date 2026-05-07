module github.com/buckedunicorn/grok/examples/discord-bot

go 1.26.3

// Use the in-repo grok module rather than a published version. If you copy
// this example into a standalone repo, drop the replace directive and bump
// the require to a real tag.
replace github.com/buckedunicorn/grok => ../..

require (
	github.com/buckedunicorn/grok v0.0.0
	github.com/bwmarrin/discordgo v0.29.0
)

require (
	github.com/gorilla/websocket v1.4.2 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4 // indirect
	google.golang.org/grpc v1.80.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
