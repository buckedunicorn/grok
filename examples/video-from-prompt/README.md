# video-from-prompt

Generates a video from a text description and polls until complete.

## Run

```sh
XAI_API_KEY=... go run ./examples/video-from-prompt
```

## What it shows

- Submitting a `videos.GenerateRequest` with only `Model`, `Prompt`, and `Duration`
- Receiving a `request_id` (generation is async)
- Polling with `Videos.Wait(ctx, requestID, 5*time.Second)` until `status == "done"`
- Reading the output `Video.URL` from the final `VideoResult`

Typical generation time is 60–120 seconds. The context deadline controls the maximum wait.
