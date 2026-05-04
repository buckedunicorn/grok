# video-edit

Edits an existing video using a text prompt.

## Run

```sh
XAI_API_KEY=... go run ./examples/video-edit <video-url> [prompt]
```

The video URL must be a publicly accessible MP4. The prompt defaults to a night-scene transformation if omitted.

## What it shows

- Calling `Videos.Edit` with a source video URL and editing instructions
- Polling with `Videos.Wait` for the async result
