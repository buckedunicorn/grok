# voice/ttsstream

WebSocket client for streaming text-to-speech. Send text in pieces, receive audio frames as the model produces them. Lower time-to-first-byte than the HTTP `voice.Client.TextToSpeech` for long inputs.

```go
import "github.com/buckedunicorn/grok/voice/ttsstream"
```

## Surface

| Function / type | Role |
|---|---|
| `Dial(ctx, apiKey, opts...)` | Opens the WebSocket; configure with `WithVoice`, `WithLanguage`, `WithCodec`, `WithSampleRate`, `WithOptimizeLatency`, `WithTextNormalization` |
| `Stream.SendText(ctx, text)` | Push a chunk of text |
| `Stream.Done(ctx)` | Signal that no more text is coming so the server can flush |
| `Stream.Audio()` | Channel of audio byte slices in the requested codec |
| `Stream.Err()` | Returns any error after `Audio()` closes |
| `Stream.Close()` | Tears down the connection |

## Example

```go
s, err := ttsstream.Dial(ctx, os.Getenv("XAI_API_KEY"),
    ttsstream.WithVoice("alloy"),
    ttsstream.WithCodec("mp3"))
if err != nil { return err }
defer s.Close()

go func() {
    for chunk := range s.Audio() {
        speaker.Write(chunk)
    }
}()

for _, sentence := range sentences {
    if err := s.SendText(ctx, sentence); err != nil { return err }
}
if err := s.Done(ctx); err != nil { return err }
if err := s.Err(); err != nil { return err }
```

## See also

- [`voice`](..) for the HTTP `TextToSpeech` endpoint when you can wait for the full clip.
- [`voice/realtime`](../realtime) for bidirectional voice (audio in, audio out, tool calls) on a single connection.
