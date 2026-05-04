# stt

Speech-to-text transcription of an audio file.

## Run

```sh
XAI_API_KEY=... go run ./examples/stt /path/to/audio.mp3
```

## What it shows

- Opening a file and passing it as `io.Reader` to `Voice.Transcribe`
- Reading `Transcript.Text`, duration, and detected language

Supported audio formats: mp3, wav, ogg, flac, m4a, and raw PCM/mulaw/alaw (with `AudioFormat` and `SampleRate`).
