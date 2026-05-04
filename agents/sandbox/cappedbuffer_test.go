package sandbox

import (
	"strings"
	"testing"
)

// cappedBuffer is internal; this test lives in the sandbox package so
// it can construct one directly without requiring docker to exercise
// the larger Run path.
func TestCappedBuffer_capsAtLimit(t *testing.T) {
	b := newCappedBuffer(16)
	for range 100 {
		_, _ = b.Write([]byte("0123456789"))
	}
	out := b.Bytes()
	// 16 bytes of payload + the literal "[truncated]" marker plus
	// surrounding newlines (~13 bytes). 64 is a generous upper bound.
	if got := len(out); got > 80 {
		t.Errorf("len = %d, want <= 80", got)
	}
	if !strings.Contains(string(out), "[truncated]") {
		t.Errorf("missing [truncated] marker in output %q", out)
	}
}

func TestCappedBuffer_negativeCapDisables(t *testing.T) {
	b := newCappedBuffer(-1)
	for range 100 {
		_, _ = b.Write([]byte("payload"))
	}
	if b.truncated {
		t.Error("negative cap should not truncate")
	}
}

func TestCappedBuffer_afterTruncationDropsSilently(t *testing.T) {
	b := newCappedBuffer(8)
	_, _ = b.Write([]byte("0123456789abcdef"))
	first := len(b.Bytes())
	_, _ = b.Write([]byte("more"))
	if len(b.Bytes()) != first {
		t.Errorf("write after truncation grew buffer from %d to %d", first, len(b.Bytes()))
	}
}
