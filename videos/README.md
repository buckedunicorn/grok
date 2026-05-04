# videos

`POST /v1/videos/{generations,edits,extensions}` and `GET /v1/videos/{request_id}`. Asynchronous video generation, with a polling helper.

```go
import "github.com/buckedunicorn/grok/videos"
```

## How it works

Video generation is asynchronous. Submission methods return a `request_id`; the result becomes available later via `GetResult`. `Wait` is a convenience that polls until the job completes.

## Surface

| API | Purpose |
|---|---|
| `Client.Generate` | Submit a text-to-video or image-to-video job. Returns `request_id` |
| `Client.Edit` | Submit a video-to-video edit. Returns `request_id` |
| `Client.Extend` | Submit an extension of an existing video. Returns `request_id` |
| `Client.GetResult` | Fetch the current state for a `request_id` (status, progress, video URL when done) |
| `Client.Wait` | Polls `GetResult` at the given interval until status is `done` or `failed` |

## Example

```go
requestID, err := client.Videos.Generate(ctx, &videos.GenerateRequest{
    Model:       "grok-video-1",
    Prompt:      "A slow zoom-in on a cosmic nebula",
    AspectRatio: "16:9",
})
if err != nil { return err }

result, err := client.Videos.Wait(ctx, requestID, 5*time.Second)
if err != nil { return err }
if result.Status == "done" {
    fmt.Println(result.Video.URL)
}
```

For long-running jobs that should not block the calling goroutine, fork your own polling loop instead of using `Wait`. The [`discord.GenerateVideoTool`](../discord) shows the async-poll-and-post pattern in production.

## Examples directory

- [`examples/video-from-prompt`](../examples/video-from-prompt)
- [`examples/video-from-image`](../examples/video-from-image)
- [`examples/video-edit`](../examples/video-edit)
- [`examples/video-extend`](../examples/video-extend)
