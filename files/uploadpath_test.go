package files_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestFiles_UploadPath_success(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "hello.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, c := newFilesClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/files" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		r.ParseMultipartForm(1 << 20)
		_, fh, err := r.FormFile("file")
		if err != nil {
			t.Errorf("no file part: %v", err)
		}
		if fh.Filename != "hello.txt" {
			t.Errorf("unexpected filename: %s", fh.Filename)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": "file-abc", "filename": "hello.txt",
		})
	})
	defer srv.Close()

	f, err := c.UploadPath(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.ID != "file-abc" {
		t.Errorf("unexpected file ID: %s", f.ID)
	}
}

func TestFiles_UploadPath_fileNotFound(t *testing.T) {
	srv, c := newFilesClient(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for a missing file")
	})
	defer srv.Close()

	_, err := c.UploadPath(context.Background(), "/nonexistent/path/file.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
