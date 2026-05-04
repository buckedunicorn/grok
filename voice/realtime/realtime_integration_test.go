package realtime_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/voice/realtime"
)

func apiKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("XAI_API_KEY")
	if key == "" {
		t.Skip("XAI_API_KEY not set")
	}
	return key
}

func TestIntegration_DialAndClose(t *testing.T) {
	key := apiKey(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sess, err := realtime.Dial(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	events, err := sess.Listen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the session.created event to confirm the connection is live.
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("event channel closed before session.created")
			}
			t.Logf("event: %s", ev.Type)
			if ev.Type == "session.created" {
				return
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for session.created")
		}
	}
}
