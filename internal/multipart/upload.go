package multipart

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
)

// Build constructs a multipart/form-data body from fields and an optional file.
// fields values may be string or []byte. file may be nil.
// Returns the body buffer and content-type header value.
//
// The file part is created via mime/multipart.Writer.CreateFormFile,
// which produces an RFC 7578 / RFC 5987 compliant Content-Disposition
// header. Any quotes in filename are escaped by the standard library
// rather than by Go's %q.
func Build(fields map[string]string, fileField, filename string, file io.Reader) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, "", fmt.Errorf("multipart field %q: %w", k, err)
		}
	}

	if file != nil {
		fw, err := w.CreateFormFile(fileField, filename)
		if err != nil {
			return nil, "", err
		}
		if _, err := io.Copy(fw, file); err != nil {
			return nil, "", err
		}
	}

	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}
