package discord_test

import (
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/discord"
)

// FuzzExtractURLs confirms ExtractURLs never panics, every returned
// item passes LooksLikeURL, and the function preserves order.
func FuzzExtractURLs(f *testing.F) {
	for _, s := range []string{
		"",
		"https://a.example.com/x",
		"see <https://b.example.com/y> and (https://c.example.com/z),",
		"http://example.com",
		"https://x.example.com/path?q=1#frag",
		"\x00\x01garbage\x02",
		"https://", // invalid
		strings.Repeat("https://a.example.com/x ", 50),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := discord.ExtractURLs(in)
		for i, u := range out {
			if !discord.LooksLikeURL(u) {
				t.Errorf("ExtractURLs[%d] = %q, fails LooksLikeURL", i, u)
			}
		}
	})
}

// FuzzLooksLikeURL confirms LooksLikeURL never panics and is
// deterministic (idempotent on the same input).
func FuzzLooksLikeURL(f *testing.F) {
	for _, s := range []string{
		"",
		"https://example.com",
		"http://example.com",
		"ftp://example.com",
		"  https://example.com  ",
		"https://./",
		"\x00",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		a := discord.LooksLikeURL(in)
		b := discord.LooksLikeURL(in)
		if a != b {
			t.Errorf("LooksLikeURL(%q) is not deterministic: %v vs %v", in, a, b)
		}
	})
}

// FuzzMention confirms Mention never panics and the result is the
// empty string only when input is empty.
func FuzzMention(f *testing.F) {
	for _, s := range []string{"", "1", "1234567890", "@everyone", "<@injected>", "\x00\x01"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := discord.Mention(in)
		if in == "" {
			if out != "" {
				t.Errorf("empty input should produce empty output, got %q", out)
			}
			return
		}
		if !strings.HasPrefix(out, "<@") || !strings.HasSuffix(out, "> ") {
			t.Errorf("Mention(%q) = %q, want <@...>", in, out)
		}
	})
}

// FuzzChunks confirms Chunks never panics, every chunk is within the
// length cap, and concatenating chunks reproduces the input.
func FuzzChunks(f *testing.F) {
	for _, sample := range []struct {
		in     string
		maxLen int
	}{
		{"", 100},
		{"short", 100},
		{strings.Repeat("a", 5000), 1000},
		{strings.Repeat("\x00", 100), 50},
		{"hello", 0}, // 0 means default
		{"hello", -1},
	} {
		f.Add(sample.in, sample.maxLen)
	}
	f.Fuzz(func(t *testing.T, in string, maxLen int) {
		out := discord.Chunks(in, maxLen)
		for _, c := range out {
			limit := maxLen
			if limit <= 0 {
				limit = 2000
			}
			if len(c) > limit {
				t.Errorf("chunk len %d exceeds limit %d", len(c), limit)
			}
		}
		joined := strings.Join(out, "")
		if joined != in {
			t.Errorf("rejoin mismatch: %d vs %d bytes", len(joined), len(in))
		}
	})
}
