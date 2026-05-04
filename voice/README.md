# voice

Text-to-speech, speech-to-text, and bidirectional realtime voice. The HTTP endpoints live in this package; the WebSocket-based streaming surfaces are in sub-packages.

```go
import "github.com/buckedunicorn/grok/voice"
```

## Surface

| API | Purpose |
|---|---|
| `Client.TextToSpeech` | One-shot TTS over HTTP. Returns audio bytes |
| `Client.ListVoices`, `Client.GetVoice` | Voice catalog |
| `Client.Transcribe` | One-shot STT over HTTP (multipart upload of an audio file or URL) |
| `Client.CreateEphemeralToken` | Short-lived token for realtime voice sessions |

## Sub-packages

| Sub-package | Purpose |
|---|---|
| [`voice/realtime`](./realtime) | Bidirectional voice conversation over WebSocket. `Session.Update`, `AppendAudio`, `CommitAudio`, `RequestResponse`, `Listen`, `Close` |
| [`voice/ttsstream`](./ttsstream) | Streaming text-to-speech over WebSocket. Send text, receive audio chunks |
| [`voice/sttstream`](./sttstream) | Streaming speech-to-text over WebSocket. Send audio, receive transcript events |

## Example

```go
audio, err := client.Voice.TextToSpeech(ctx, &voice.TTSRequest{
    Text:    "Hello from Grok",
    VoiceID: "ainsley",
    OutputFormat: &voice.OutputFormat{Codec: "mp3", SampleRate: 24000},
})
if err != nil { return err }
_ = os.WriteFile("hello.mp3", audio, 0644)
```

For STT:

```go
file, _ := os.Open("speech.wav")
defer file.Close()
tx, err := client.Voice.Transcribe(ctx, &voice.STTRequest{
    File:     file,
    Language: "en",
})
if err != nil { return err }
fmt.Println(tx.Text)
```

Realtime sessions are long-lived; the caller owns the lifecycle. See the realtime sub-package's godoc for the full message flow.

## Examples directory

- [`examples/tts`](../examples/tts)
- [`examples/stt`](../examples/stt)
