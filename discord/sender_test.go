package discord_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/discord"
)

// fakeSender records Send/Typing calls for assertions. Implements
// discord.Sender.
type fakeSender struct {
	sends  atomic.Int32
	typing atomic.Int32
	last   atomic.Pointer[string]
}

func (f *fakeSender) Send(_, content string) error {
	f.sends.Add(1)
	f.last.Store(&content)
	return nil
}

func (f *fakeSender) Typing(string) error {
	f.typing.Add(1)
	return nil
}

func TestKeepTyping_pingsImmediatelyAndRepeats(t *testing.T) {
	f := &fakeSender{}
	stop := discord.KeepTyping(f, "ch1", 25*time.Millisecond)
	defer stop()

	// First ping is synchronous-ish; allow the goroutine to schedule.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) && f.typing.Load() < 3 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := f.typing.Load(); got < 3 {
		t.Fatalf("typing pings = %d, want at least 3 within 200ms (interval=25ms)", got)
	}
}

func TestKeepTyping_stopHaltsPings(t *testing.T) {
	f := &fakeSender{}
	stop := discord.KeepTyping(f, "ch1", 10*time.Millisecond)
	time.Sleep(35 * time.Millisecond)
	stop()
	stable := f.typing.Load()

	time.Sleep(40 * time.Millisecond)
	if got := f.typing.Load(); got != stable {
		t.Errorf("ping count grew after stop: was %d, now %d", stable, got)
	}
}

func TestKeepTyping_stopIsIdempotent(t *testing.T) {
	f := &fakeSender{}
	stop := discord.KeepTyping(f, "ch1", time.Hour) // never tick
	stop()
	stop() // must not panic / double-close
}

func TestMention(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"123", "<@123> "},
		{"", ""},
	} {
		if got := discord.Mention(tc.in); got != tc.want {
			t.Errorf("Mention(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLooksLikeURL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"https://example.com/foo", true},
		{"http://example.com", true},
		{"https://example.com", true},
		{"   https://example.com  ", true},
		{"ftp://example.com", false},
		{"example.com", false},
		{"https://", false},
		{"https://./", false},
		{"", false},
	} {
		if got := discord.LooksLikeURL(tc.in); got != tc.want {
			t.Errorf("LooksLikeURL(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLooksLikeImageURL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"https://example.com/cat.png", true},
		{"https://example.com/CAT.PNG", true},
		{"https://example.com/cat.jpg?token=xyz", true},
		{"https://example.com/cat.gif", true},
		{"https://example.com/cat.webp", true},
		{"https://example.com/cat.mp4", false},
		{"https://example.com/cat", false},
		{"not a url", false},
	} {
		if got := discord.LooksLikeImageURL(tc.in); got != tc.want {
			t.Errorf("LooksLikeImageURL(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLooksLikeVideoURL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"https://example.com/clip.mp4", true},
		{"https://example.com/clip.MP4?sig=abc", true},
		{"https://example.com/clip.webm", true},
		{"https://example.com/clip.mov", true},
		{"https://vidgen.x.ai/abc", true}, // host pattern even without ext
		{"https://example.com/clip.png", false},
		{"https://example.com/clip", false},
	} {
		if got := discord.LooksLikeVideoURL(tc.in); got != tc.want {
			t.Errorf("LooksLikeVideoURL(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestExtractURLs(t *testing.T) {
	in := "see <https://example.com/a> and (https://b.example.com/x)! also https://c.example.com/y."
	got := discord.ExtractURLs(in)
	want := []string{
		"https://example.com/a",
		"https://b.example.com/x",
		"https://c.example.com/y",
	}
	if len(got) != len(want) {
		t.Fatalf("ExtractURLs len = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("ExtractURLs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExtractURLs_empty(t *testing.T) {
	if got := discord.ExtractURLs(""); got != nil {
		t.Errorf("ExtractURLs(\"\") = %v, want nil", got)
	}
	if got := discord.ExtractURLs("no links here just words"); got != nil {
		t.Errorf("ExtractURLs returned %v on no-link input", got)
	}
}

func TestChannelContext_roundTrip(t *testing.T) {
	ctx := discord.WithChannelContext(context.Background(), "ch-42")
	if got := discord.ChannelFromContext(ctx); got != "ch-42" {
		t.Errorf("ChannelFromContext = %q, want %q", got, "ch-42")
	}
	if got := discord.ChannelFromContext(context.Background()); got != "" {
		t.Errorf("missing key should return \"\", got %q", got)
	}
}

func TestChannelContext_typeMismatchReturnsEmpty(t *testing.T) {
	// Setting a value with a different (unrelated) key should not be
	// returned by ChannelFromContext.
	type otherKey struct{}
	ctx := context.WithValue(context.Background(), otherKey{}, "ch-x")
	if got := discord.ChannelFromContext(ctx); got != "" {
		t.Errorf("unrelated key bled through: got %q", got)
	}
}

func TestExtractURLs_doesNotPickUpFragments(t *testing.T) {
	// Ensure the trim set is right, trailing closing punctuation
	// strips, leading scheme stays.
	got := discord.ExtractURLs("(https://x.example.com),")
	if len(got) != 1 || got[0] != "https://x.example.com" {
		t.Errorf("unexpected: %v", got)
	}
	// Mid-string punctuation must NOT be stripped (path chars).
	got = discord.ExtractURLs("https://x.example.com/a,b")
	if len(got) != 1 || !strings.HasPrefix(got[0], "https://x.example.com/a") {
		t.Errorf("middle-comma stripped erroneously: %v", got)
	}
}
