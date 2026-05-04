package multipart_test

import (
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	mp "github.com/buckedunicorn/grok/internal/multipart"
)

func TestBuild_withFile(t *testing.T) {
	fields := map[string]string{"language": "en", "model": "whisper"}
	fileContent := "fake audio bytes"

	buf, contentType, err := mp.Build(fields, "file", "audio.wav", strings.NewReader(fileContent))
	if err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("body is empty")
	}

	// Parse the multipart body to verify structure.
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("ParseMediaType: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Errorf("media type = %q, want multipart/form-data", mediaType)
	}

	mr := multipart.NewReader(buf, params["boundary"])
	got := map[string]string{}
	var gotFilename string
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		if fn := p.FileName(); fn != "" {
			gotFilename = fn
			continue
		}
		b, _ := io.ReadAll(p)
		got[p.FormName()] = string(b)
	}

	for k, v := range fields {
		if got[k] != v {
			t.Errorf("field %q = %q, want %q", k, got[k], v)
		}
	}
	if gotFilename != "audio.wav" {
		t.Errorf("filename = %q, want %q", gotFilename, "audio.wav")
	}
}

func TestBuild_noFile(t *testing.T) {
	fields := map[string]string{"key": "value"}

	buf, contentType, err := mp.Build(fields, "file", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("ParseMediaType: %v", err)
	}

	mr := multipart.NewReader(buf, params["boundary"])
	count := 0
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		if p.FormName() == "key" {
			b, _ := io.ReadAll(p)
			if string(b) != "value" {
				t.Errorf("field value = %q, want %q", string(b), "value")
			}
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 field part, got %d", count)
	}
}

func TestBuild_contentTypeHasBoundary(t *testing.T) {
	_, contentType, err := mp.Build(nil, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
		t.Errorf("contentType = %q, want multipart/form-data; boundary=...", contentType)
	}
}
