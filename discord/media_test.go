package discord_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/discord"
	"github.com/buckedunicorn/grok/images"
	"github.com/buckedunicorn/grok/internal/transport"
	"github.com/buckedunicorn/grok/videos"
)

// ----------------------------------------------------------------------------
// URLRing
// ----------------------------------------------------------------------------

func TestURLRing_addAndRecent(t *testing.T) {
	r := discord.NewURLRing(3)
	r.Add("ch", "a")
	r.Add("ch", "b")
	r.Add("ch", "c")
	if got := r.Recent("ch"); got != "c" {
		t.Errorf("Recent = %q, want %q", got, "c")
	}
}

func TestURLRing_evictsOldest(t *testing.T) {
	r := discord.NewURLRing(2)
	r.Add("ch", "a")
	r.Add("ch", "b")
	r.Add("ch", "c") // evicts "a"
	if got := r.Recent("ch"); got != "c" {
		t.Errorf("Recent after eviction = %q, want %q", got, "c")
	}
	r.Add("ch", "d") // evicts "b"
	if got := r.Recent("ch"); got != "d" {
		t.Errorf("Recent = %q, want %q", got, "d")
	}
}

func TestURLRing_separateChannels(t *testing.T) {
	r := discord.NewURLRing(4)
	r.Add("a", "1")
	r.Add("b", "2")
	if got := r.Recent("a"); got != "1" {
		t.Errorf("ch a = %q", got)
	}
	if got := r.Recent("b"); got != "2" {
		t.Errorf("ch b = %q", got)
	}
}

func TestURLRing_resetClearsChannel(t *testing.T) {
	r := discord.NewURLRing(2)
	r.Add("a", "1")
	r.Add("b", "2")
	r.Reset("a")
	if got := r.Recent("a"); got != "" {
		t.Errorf("Reset didn't clear ch a, got %q", got)
	}
	if got := r.Recent("b"); got != "2" {
		t.Errorf("Reset of ch a leaked to ch b: %q", got)
	}
}

func TestURLRing_emptyAndZeroValues(t *testing.T) {
	r := discord.NewURLRing(2)
	if got := r.Recent("never-added"); got != "" {
		t.Errorf("unknown channel should be empty, got %q", got)
	}
	r.Add("ch", "")  // empty URL ignored
	r.Add("", "url") // empty channel ignored
	if got := r.Recent("ch"); got != "" {
		t.Errorf("empty url should not be stored, got %q", got)
	}

	// nil ring: Add/Recent/Reset should no-op gracefully.
	var nilRing *discord.URLRing
	nilRing.Add("ch", "x")
	nilRing.Reset("ch")
	if got := nilRing.Recent("ch"); got != "" {
		t.Errorf("nil ring should be empty, got %q", got)
	}
}

func TestURLRing_concurrentSafety(t *testing.T) {
	// Smoke test under -race; not measuring contention.
	r := discord.NewURLRing(50)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				r.Add("ch", "url")
				_ = r.Recent("ch")
			}
		}(w)
	}
	wg.Wait()
}

// ----------------------------------------------------------------------------
// MessageContext
// ----------------------------------------------------------------------------

func TestMessageContext_roundTrip(t *testing.T) {
	mc := discord.MessageContext{
		UserID:        "u-1",
		FallbackImage: "https://example.com/i.png",
		FallbackVideo: "https://example.com/v.mp4",
	}
	ctx := discord.WithMessageContext(context.Background(), mc)
	got := discord.MessageContextFrom(ctx)
	if got != mc {
		t.Errorf("round-trip changed value: got %+v want %+v", got, mc)
	}
}

func TestMessageContext_missingReturnsZero(t *testing.T) {
	got := discord.MessageContextFrom(context.Background())
	if got != (discord.MessageContext{}) {
		t.Errorf("missing context should be zero value, got %+v", got)
	}
}

// ----------------------------------------------------------------------------
// GenerateImageTool / EditImageTool
// ----------------------------------------------------------------------------

// fakeImagesServer responds to /v1/images/generations and /v1/images/edits
// with a single fake URL plus a revised_prompt. captures the request body
// for the latest call.
type fakeImagesServer struct {
	*httptest.Server
	mu       sync.Mutex
	lastBody []byte
	lastPath string
}

func newFakeImagesServer(t *testing.T) *fakeImagesServer {
	t.Helper()
	f := &fakeImagesServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readAll(t, r.Body)
		f.mu.Lock()
		f.lastBody = body
		f.lastPath = r.URL.Path
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"url": "https://gen.example.com/img.png", "revised_prompt": "a fluffy cat"},
			},
		})
	}))
	return f
}

func TestGenerateImageTool_postsMediaWithMention(t *testing.T) {
	srv := newFakeImagesServer(t)
	defer srv.Close()
	tr := transport.NewInsecure("k", srv.URL, srv.Client())
	imgs := images.NewClient(tr)

	sender := &fakeSender{}
	tool, handler := discord.GenerateImageTool(imgs, sender)
	if tool.Function.Name != "generate_image" {
		t.Errorf("default name = %q", tool.Function.Name)
	}

	ctx := discord.WithChannelContext(context.Background(), "ch-1")
	ctx = discord.WithMessageContext(ctx, discord.MessageContext{UserID: "u-9"})
	out, err := handler(ctx, `{"prompt":"a fluffy cat","aspect_ratio":"1:1"}`)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if got["image_url"] != "https://gen.example.com/img.png" {
		t.Errorf("image_url = %v", got["image_url"])
	}
	if posted, _ := got["posted"].(bool); !posted {
		t.Errorf("posted = false, expected true")
	}
	if sender.sends.Load() != 1 {
		t.Errorf("send count = %d, want 1", sender.sends.Load())
	}
	if last := sender.last.Load(); last == nil {
		t.Fatal("sender did not record a message")
	} else if !strings.HasPrefix(*last, "<@u-9> ") {
		t.Errorf("sender message %q missing user mention prefix", *last)
	}
}

func TestGenerateImageTool_skipPostWithNilSender(t *testing.T) {
	srv := newFakeImagesServer(t)
	defer srv.Close()
	tr := transport.NewInsecure("k", srv.URL, srv.Client())
	imgs := images.NewClient(tr)

	_, handler := discord.GenerateImageTool(imgs, nil)
	ctx := discord.WithChannelContext(context.Background(), "ch-1")
	out, err := handler(ctx, `{"prompt":"a cat"}`)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal([]byte(out), &got)
	if posted, _ := got["posted"].(bool); posted {
		t.Errorf("expected posted=false with nil sender")
	}
}

func TestEditImageTool_usesFallbackWhenModelOmits(t *testing.T) {
	srv := newFakeImagesServer(t)
	defer srv.Close()
	tr := transport.NewInsecure("k", srv.URL, srv.Client())
	imgs := images.NewClient(tr)

	_, handler := discord.EditImageTool(imgs, nil)

	ctx := discord.WithChannelContext(context.Background(), "ch-1")
	ctx = discord.WithMessageContext(ctx, discord.MessageContext{
		FallbackImage: "https://store.example.com/orig.png",
	})
	if _, err := handler(ctx, `{"prompt":"variant"}`); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !strings.Contains(string(srv.lastBody), "store.example.com/orig.png") {
		t.Errorf("fallback image not forwarded; body = %s", srv.lastBody)
	}
}

func TestEditImageTool_errorsWhenNoReference(t *testing.T) {
	srv := newFakeImagesServer(t)
	defer srv.Close()
	tr := transport.NewInsecure("k", srv.URL, srv.Client())
	imgs := images.NewClient(tr)

	_, handler := discord.EditImageTool(imgs, nil)
	ctx := discord.WithChannelContext(context.Background(), "ch-1")
	out, err := handler(ctx, `{"prompt":"variant"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no reference image") {
		t.Errorf("expected error response, got %q", out)
	}
}

// ----------------------------------------------------------------------------
// GenerateVideoTool / ExtendVideoTool
// ----------------------------------------------------------------------------

// fakeVideosServer: returns request_id on POST, and progresses
// status=pending → done on subsequent GET polls. Also serves binary
// bytes when an image URL is fetched (for the data-URI inline path).
type fakeVideosServer struct {
	*httptest.Server
	mu            sync.Mutex
	state         string // "pending" / "done"
	lastVideoBody []byte
}

func newFakeVideosServer(t *testing.T) *fakeVideosServer {
	t.Helper()
	f := &fakeVideosServer{state: "pending"}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			f.mu.Lock()
			f.lastVideoBody = readAll(t, r.Body)
			f.state = "done" // first poll will return done
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "rid-123"})
		case http.MethodGet:
			f.mu.Lock()
			s := f.state
			f.mu.Unlock()
			resp := map[string]any{"status": s, "progress": 100}
			if s == "done" {
				resp["video"] = map[string]any{"url": "https://gen.example.com/v.mp4"}
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	return f
}

func TestGenerateVideoTool_returnsRequestIDAndPostsOnDone(t *testing.T) {
	srv := newFakeVideosServer(t)
	defer srv.Close()
	tr := transport.NewInsecure("k", srv.URL, srv.Client())
	vids := videos.NewClient(tr)

	sender := &fakeSender{}
	mem := discord.NewURLRing(4)
	_, handler := discord.GenerateVideoTool(vids, sender, mem,
		discord.WithVideoPollInterval(5*time.Millisecond),
		discord.WithVideoPollTimeout(2*time.Second),
	)

	ctx := discord.WithChannelContext(context.Background(), "ch-x")
	ctx = discord.WithMessageContext(ctx, discord.MessageContext{UserID: "u-7"})
	out, err := handler(ctx, `{"prompt":"a galaxy spinning"}`)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal([]byte(out), &got)
	if got["request_id"] != "rid-123" {
		t.Errorf("request_id = %v", got["request_id"])
	}
	if got["status"] != "generating" {
		t.Errorf("status = %v", got["status"])
	}
	if got["mode"] != "text-to-video" {
		t.Errorf("mode = %v", got["mode"])
	}

	// Wait for the poll goroutine to post the done message.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && sender.sends.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if sender.sends.Load() == 0 {
		t.Fatal("poll goroutine never posted a message")
	}
	if last := sender.last.Load(); last == nil {
		t.Fatal("sender recorded no message")
	} else if !strings.Contains(*last, "https://gen.example.com/v.mp4") {
		t.Errorf("posted message lacked video URL: %q", *last)
	} else if !strings.HasPrefix(*last, "<@u-7> ") {
		t.Errorf("posted message lacked mention: %q", *last)
	}

	// Memory should have the URL stored for ExtendVideoTool fallback.
	if got := mem.Recent("ch-x"); got != "https://gen.example.com/v.mp4" {
		t.Errorf("memory.Recent = %q", got)
	}
}

func TestExtendVideoTool_fallsBackToMemory(t *testing.T) {
	srv := newFakeVideosServer(t)
	defer srv.Close()
	tr := transport.NewInsecure("k", srv.URL, srv.Client())
	vids := videos.NewClient(tr)

	mem := discord.NewURLRing(2)
	mem.Add("ch-y", "https://prior.example.com/v.mp4")

	_, handler := discord.ExtendVideoTool(vids, nil, mem,
		discord.WithVideoPollInterval(time.Hour), // we only care about the submit path
	)
	ctx := discord.WithChannelContext(context.Background(), "ch-y")
	if _, err := handler(ctx, `{"prompt":"continue"}`); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !strings.Contains(string(srv.lastVideoBody), "prior.example.com/v.mp4") {
		t.Errorf("extend didn't pick up memory fallback; body = %s", srv.lastVideoBody)
	}
}

func TestExtendVideoTool_errorsWhenNoVideo(t *testing.T) {
	tr := transport.NewInsecure("k", "http://unused", http.DefaultClient)
	vids := videos.NewClient(tr)
	_, handler := discord.ExtendVideoTool(vids, nil, nil)

	out, err := handler(context.Background(), `{"prompt":"continue"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no video to extend") {
		t.Errorf("expected error, got %q", out)
	}
}

// ----------------------------------------------------------------------------
// FetchAsDataURI
// ----------------------------------------------------------------------------

func TestFetchAsDataURI_inlinesBytes(t *testing.T) {
	body := []byte("\x89PNG\r\n\x1a\nfakebytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png; charset=utf-8")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	// httptest binds to loopback HTTP; opt out of the SSRF guard for
	// the test fixture.
	got, err := discord.FetchAsDataURI(context.Background(), srv.URL,
		discord.WithFetchAllowHTTP(),
		discord.WithFetchAllowPrivateAddresses())
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("URI prefix = %q, want %q", got[:min(len(got), len(prefix))], prefix)
	}
	encoded := strings.TrimPrefix(got, prefix)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("not base64: %v", err)
	}
	if string(decoded) != string(body) {
		t.Errorf("decoded mismatch: got %q want %q", decoded, body)
	}
}

func TestFetchAsDataURI_capsSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer srv.Close()

	_, err := discord.FetchAsDataURI(context.Background(), srv.URL,
		discord.WithFetchMaxBytes(1024),
		discord.WithFetchAllowHTTP(),
		discord.WithFetchAllowPrivateAddresses())
	if err == nil {
		t.Fatal("expected size-cap error")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error = %v, want size-cap message", err)
	}
}

func TestFetchAsDataURI_httpErrorStatusReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := discord.FetchAsDataURI(context.Background(), srv.URL,
		discord.WithFetchAllowHTTP(),
		discord.WithFetchAllowPrivateAddresses())
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 error, got %v", err)
	}
}

// SSRF guard tests.

func TestFetchAsDataURI_rejectsHTTPByDefault(t *testing.T) {
	_, err := discord.FetchAsDataURI(context.Background(), "http://example.com/")
	if err == nil || !strings.Contains(err.Error(), "scheme not allowed") {
		t.Errorf("expected scheme rejection, got %v", err)
	}
}

func TestFetchAsDataURI_rejectsLoopback(t *testing.T) {
	_, err := discord.FetchAsDataURI(context.Background(), "https://127.0.0.1/")
	if err == nil || !strings.Contains(err.Error(), "non-public destination") {
		t.Errorf("expected loopback rejection, got %v", err)
	}
}

func TestFetchAsDataURI_rejectsLinkLocal(t *testing.T) {
	_, err := discord.FetchAsDataURI(context.Background(), "https://169.254.169.254/")
	if err == nil || !strings.Contains(err.Error(), "non-public destination") {
		t.Errorf("expected link-local rejection, got %v", err)
	}
}

func TestFetchAsDataURI_rejectsPrivateRange(t *testing.T) {
	_, err := discord.FetchAsDataURI(context.Background(), "https://10.0.0.1/")
	if err == nil || !strings.Contains(err.Error(), "non-public destination") {
		t.Errorf("expected private-IP rejection, got %v", err)
	}
}

func TestFetchAsDataURI_rejectsUnsupportedScheme(t *testing.T) {
	_, err := discord.FetchAsDataURI(context.Background(), "ftp://example.com/")
	if err == nil || !strings.Contains(err.Error(), "scheme not allowed") {
		t.Errorf("expected scheme rejection, got %v", err)
	}
}

func TestFetchAsDataURI_allowListRejectsUnknownHost(t *testing.T) {
	_, err := discord.FetchAsDataURI(context.Background(), "https://1.1.1.1/",
		discord.WithFetchAllowList("cdn.example.com"))
	if err == nil || !strings.Contains(err.Error(), "allow-list") {
		t.Errorf("expected allow-list rejection, got %v", err)
	}
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func readAll(t *testing.T, r interface{ Read(p []byte) (int, error) }) []byte {
	t.Helper()
	var buf strings.Builder
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			break
		}
	}
	return []byte(buf.String())
}

// Re-declare a tiny chat tool sanity test that ensures the tools have
// the names and required-arg shape RunAgent expects.
func TestMediaTools_haveExpectedNamesAndRequiredFields(t *testing.T) {
	type expect struct {
		name     string
		required string
	}
	for _, tc := range []struct {
		fn   func() (chat.Tool, chat.Handler)
		want expect
	}{
		{func() (chat.Tool, chat.Handler) {
			return discord.GenerateImageTool(nil, nil)
		}, expect{"generate_image", "prompt"}},
		{func() (chat.Tool, chat.Handler) {
			return discord.EditImageTool(nil, nil)
		}, expect{"edit_image", "prompt"}},
		{func() (chat.Tool, chat.Handler) {
			return discord.GenerateVideoTool(nil, nil, nil)
		}, expect{"generate_video", "prompt"}},
		{func() (chat.Tool, chat.Handler) {
			return discord.ExtendVideoTool(nil, nil, nil)
		}, expect{"extend_video", "prompt"}},
	} {
		tool, _ := tc.fn()
		if tool.Function.Name != tc.want.name {
			t.Errorf("tool name = %q, want %q", tool.Function.Name, tc.want.name)
			continue
		}
		params, _ := tool.Function.Parameters.(map[string]any)
		req, _ := params["required"].([]string)
		if len(req) == 0 || req[0] != tc.want.required {
			t.Errorf("%s required[0] = %v, want %q", tc.want.name, req, tc.want.required)
		}
	}
}
