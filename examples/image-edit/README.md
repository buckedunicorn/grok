# image-edit

Modifies an existing image using a text prompt.

## Run

```sh
XAI_API_KEY=... go run ./examples/image-edit <image-url> [prompt]
```

The image URL must be publicly accessible. The prompt defaults to adding a dramatic sky if omitted.

## What it shows

- Calling `Images.Edit` with an `EditRequest` containing an `ImageSource` URL and editing prompt
- Reading the edited image URL (or base64 JSON) from `ImageResponse.Data`

For multi-reference editing, populate `EditRequest.Images` (a slice of `ImageSource`) instead of the single `Image` field.
