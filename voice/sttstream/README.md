# voice/sttstream

WebSocket client for streaming speech-to-text. Send audio frames in real time, receive transcript events as the model produces them. Lower time-to-first-word than the HTTP `voice.Client.Transcribe` for long inputs and the right choice for live captioning.

```go
import "github.com/buckedunicorn/grok/voice/sttstream"
```

## Surface

| Function / type | Role |
|---|---|
| `Dial(ctx, apiKey, opts...)` | Opens the WebSocket; configure with `WithSampleRate`, `WithEncoding`, `WithInterimResults`, `WithEndpointing`, `WithLanguage`, `WithMultichannel`, `WithChannels`, `WithDiarize` |
| `Stream.Ready()` | Closes when the server has accepted the stream config; safe to send audio after |
| `Stream.SendAudio(ctx, frame)` | Push a chunk of raw audio (PCM, mu-law, A-law) |
| `Stream.Done(ctx)` | Signal that no more audio is coming so the server can flush |
| `Stream.Listen(ctx)` | Channel of `TranscriptEvent` values |
| `Stream.Close()` | Tears down the connection |
| `TranscriptEvent` | One transcript update: text, words, optional channel breakdown, optional error |

Set `WithInterimResults(true)` to receive incremental partial transcripts before each utterance is final; set `WithEndpointing(ms)` to control how long of a silence triggers an utterance boundary.

## Example

```go
s, err := sttstream.Dial(ctx, os.Getenv("XAI_API_KEY"),
    sttstream.WithEncoding("pcm"),
    sttstream.WithSampleRate(16000),
    sttstream.WithInterimResults(true))
if err != nil { return err }
defer s.Close()
<-s.Ready()

go func() {
    for ev := range s.Listen(ctx) {
        if ev.Error != "" {
            log.Printf("stt: %s", ev.Error)
            continue
        }
        fmt.Println(ev.Text)
    }
}()

for chunk := range mic.Chunks(ctx) {
    if err := s.SendAudio(ctx, chunk); err != nil { return err }
}
if err := s.Done(ctx); err != nil { return err }
```

## See also

- [`voice`](..) for the HTTP `Transcribe` endpoint when you have a pre-recorded file.
- [`voice/realtime`](../realtime) for bidirectional voice (audio in, audio out) on a single connection.
