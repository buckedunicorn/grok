# video-extend

Extends an existing video by generating additional seconds of footage.

## Run

```sh
XAI_API_KEY=... go run ./examples/video-extend <video-url> [prompt]
```

The video URL must be a publicly accessible MP4. The prompt guides the continuation; it defaults to a slow pan if omitted.

## What it shows

- Calling `Videos.Extend` with a source video, a continuation prompt, and a `Duration` (1–10 s)
- Polling with `Videos.Wait` for the async result
