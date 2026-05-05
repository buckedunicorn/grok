# voice/realtime

WebSocket client for bidirectional realtime voice conversations against `wss://api.x.ai/v1/realtime`. The model listens to streaming audio, decides when the user is done, and replies with streaming audio (and optional tool calls) on the same connection.

```go
import "github.com/buckedunicorn/grok/voice/realtime"
```

## Surface

| Type / function | Role |
|---|---|
| `Dial(ctx, apiKey, opts...)` | Opens a WebSocket using a long-lived API key (server-side use) |
| `DialWithEphemeralToken(ctx, token, opts...)` | Opens a WebSocket using a short-lived token from `voice.Client.CreateEphemeralToken` (browser/edge use) |
| `Session` | The connection. Methods send events; `Listen` reads them |
| `Session.Update(ctx, config)` | Reconfigure mid-session (voice, instructions, tools, audio formats) |
| `Session.AppendAudio` / `CommitAudio` / `ClearAudio` | Stream user audio in, commit a turn, or discard the buffer |
| `Session.RequestResponse` / `CancelResponse` | Trigger a model reply or cancel a streaming one |
| `Session.SendText` / `SendToolResult` | Inject text or tool-result events |
| `Session.Listen()` | Returns a channel of decoded `Event` values |
| `Event` | One server event (audio delta, transcript delta, function call, etc.) |

## Example

```go
sess, err := realtime.Dial(ctx, os.Getenv("XAI_API_KEY"),
    realtime.WithModel("grok-voice-fast-1.0"))
if err != nil { return err }
defer sess.Close()

events, err := sess.Listen()
if err != nil { return err }

go func() {
    for ev := range events {
        if audio, err := ev.AudioDelta(); err == nil && len(audio) > 0 {
            speaker.Write(audio)
        }
    }
}()

for chunk := range mic.Chunks(ctx) {
    if err := sess.AppendAudio(ctx, chunk); err != nil { return err }
}
if err := sess.CommitAudio(ctx); err != nil { return err }
if err := sess.RequestResponse(ctx); err != nil { return err }
```

For browser or mobile clients that should not embed the long-lived API key, mint an ephemeral token with `voice.Client.CreateEphemeralToken` server-side and pass it to `DialWithEphemeralToken` from the client.

## See also

- [`voice`](..) for the HTTP-based TTS, STT, and ephemeral-token endpoints.
- [`voice/ttsstream`](../ttsstream) for streaming TTS only.
- [`voice/sttstream`](../sttstream) for streaming STT only.
