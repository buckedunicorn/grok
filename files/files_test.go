package files_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buckedunicorn/grok/files"
	"github.com/buckedunicorn/grok/internal/transport"
)

func newFilesClient(handler http.HandlerFunc) (*httptest.Server, *files.Client) {
	srv := httptest.NewServer(handler)
	t := transport.NewInsecure("test-key", srv.URL, srv.Client())
	return srv, files.NewClient(t)
}

func TestFiles_All_singlePage(t *testing.T) {
	srv, c := newFilesClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(files.FileList{
			Data: []files.File{
				{ID: "f1", Filename: "a.txt"},
				{ID: "f2", Filename: "b.txt"},
			},
		})
	})
	defer srv.Close()

	var got []string
	for f, err := range c.All(context.Background(), nil) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, f.ID)
	}
	if len(got) != 2 || got[0] != "f1" || got[1] != "f2" {
		t.Errorf("unexpected files: %v", got)
	}
}

func TestFiles_All_multiPage(t *testing.T) {
	page := 0
	srv, c := newFilesClient(func(w http.ResponseWriter, r *http.Request) {
		page++
		tok := r.URL.Query().Get("pagination_token")
		switch tok {
		case "":
			next := "page2"
			json.NewEncoder(w).Encode(files.FileList{
				Data:            []files.File{{ID: "f1"}},
				PaginationToken: &next,
			})
		case "page2":
			json.NewEncoder(w).Encode(files.FileList{
				Data: []files.File{{ID: "f2"}},
			})
		default:
			t.Errorf("unexpected pagination_token: %s", tok)
			w.WriteHeader(500)
		}
	})
	defer srv.Close()

	var got []string
	for f, err := range c.All(context.Background(), nil) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, f.ID)
	}
	if len(got) != 2 || got[0] != "f1" || got[1] != "f2" {
		t.Errorf("unexpected files: %v", got)
	}
	if page != 2 {
		t.Errorf("expected 2 page fetches, got %d", page)
	}
}

func TestFiles_All_earlyBreak(t *testing.T) {
	calls := 0
	srv, c := newFilesClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		next := "page2"
		json.NewEncoder(w).Encode(files.FileList{
			Data:            []files.File{{ID: "f1"}, {ID: "f2"}},
			PaginationToken: &next,
		})
	})
	defer srv.Close()

	count := 0
	for _, err := range c.All(context.Background(), nil) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count++
		break
	}
	if count != 1 {
		t.Errorf("expected 1 item before break, got %d", count)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 HTTP call before break, got %d", calls)
	}
}

func TestFiles_All_apiError(t *testing.T) {
	srv, c := newFilesClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"code":"internal","error":"server error"}`))
	})
	defer srv.Close()

	for _, err := range c.All(context.Background(), nil) {
		if err == nil {
			t.Fatal("expected error from iterator")
		}
		return
	}
	t.Fatal("iterator should have yielded an error")
}
