# images

`POST /v1/images/generations` and `POST /v1/images/edits`. Text-to-image and image-to-image generation.

```go
import "github.com/buckedunicorn/grok/images"
```

## Surface

| API | Purpose |
|---|---|
| `Client.Generate` | Text-to-image. Returns one or more URLs (or base64) for the generated images |
| `Client.Edit` | Image-to-image. Takes a reference image (URL or bytes) plus optional mask |

## Example

```go
resp, err := client.Images.Generate(ctx, &images.GenerateRequest{
    Model:          "grok-image-1",
    Prompt:         "A photo of an otter eating sushi",
    AspectRatio:    "16:9",
    Quality:        "high",
    Resolution:     "2k",
    ResponseFormat: "url",
})
if err != nil { return err }
fmt.Println(resp.Data[0].URL)
```

For image-to-image, pass either `Image: &ImageSource{URL: "https://..."}` or `Image: &ImageSource{Bytes: data, MIME: "image/png"}` to `Client.Edit`.

## Examples directory

- [`examples/images`](../examples/images)
- [`examples/image-edit`](../examples/image-edit)
