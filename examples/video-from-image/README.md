# video-from-image

Animates a still image into a short video (image-to-video).

## Run

```sh
XAI_API_KEY=... go run ./examples/video-from-image <image-url>
```

The image URL must be publicly accessible.

## What it shows

- Setting `Image: &videos.VideoSource{URL: imageURL}` in `GenerateRequest`
- Combining a starting image with a text prompt to control motion
- Using `Videos.Wait` to block until the async generation completes
