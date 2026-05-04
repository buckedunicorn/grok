# tts

Text-to-speech synthesis writing audio to `output.mp3`.

## Run

```sh
XAI_API_KEY=... go run ./examples/tts
```

## What it shows

- Calling `Voice.TextToSpeech` with a `TTSRequest` specifying voice and codec
- Writing raw audio bytes to a local file with `os.WriteFile`

To list available voices: call `client.Voice.ListVoices(ctx)`.
