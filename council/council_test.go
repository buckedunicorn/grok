package council_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/council"
	"github.com/buckedunicorn/grok/internal/transport"
)

func newParticipant(name string, handler http.HandlerFunc) (*httptest.Server, council.Participant) {
	srv := httptest.NewServer(handler)
	t := transport.NewInsecure("test-key", srv.URL, srv.Client())
	return srv, council.Participant{
		Name:   name,
		Client: chat.NewClient(t),
		Model:  "test-model",
	}
}

func okHandler(content string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{
				{Message: chat.Message{Role: "assistant", Content: content}},
			},
		})
	}
}

func TestRoundtable_allSucceed(t *testing.T) {
	srv1, p1 := newParticipant("alice", okHandler("answer from alice"))
	srv2, p2 := newParticipant("bob", okHandler("answer from bob"))
	defer srv1.Close()
	defer srv2.Close()

	responses, err := council.Roundtable(context.Background(), "what is 2+2?", []council.Participant{p1, p2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}
	if responses[0].Name != "alice" || responses[0].Output != "answer from alice" {
		t.Errorf("unexpected response[0]: %+v", responses[0])
	}
	if responses[1].Name != "bob" || responses[1].Output != "answer from bob" {
		t.Errorf("unexpected response[1]: %+v", responses[1])
	}
}

func TestRoundtable_oneFailsCapturedInResponse(t *testing.T) {
	srv1, p1 := newParticipant("ok", okHandler("fine"))
	srv2, p2 := newParticipant("broken", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"code":"internal","error":"boom"}`))
	})
	defer srv1.Close()
	defer srv2.Close()

	responses, err := council.Roundtable(context.Background(), "q", []council.Participant{p1, p2})
	if err != nil {
		t.Fatalf("top-level error should be nil when only one participant fails: %v", err)
	}
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}
	// find the broken one
	var broken *council.Response
	for i := range responses {
		if responses[i].Name == "broken" {
			broken = &responses[i]
		}
	}
	if broken == nil || broken.Err == nil {
		t.Errorf("expected broken participant to have Err set: %+v", broken)
	}
}

func TestRoundtable_contextCancelledPropagates(t *testing.T) {
	srv, p := newParticipant("slow", func(w http.ResponseWriter, r *http.Request) {
		// block until request context is done
		<-r.Context().Done()
		w.WriteHeader(503)
	})
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := council.Roundtable(ctx, "q", []council.Participant{p})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected Canceled, got: %v", err)
	}
}

func TestRoundtable_preservesOrder(t *testing.T) {
	names := []string{"a", "b", "c", "d"}
	var srvs []*httptest.Server
	var participants []council.Participant

	for _, name := range names {
		n := name
		srv, p := newParticipant(n, okHandler("reply from "+n))
		srvs = append(srvs, srv)
		participants = append(participants, p)
	}
	defer func() {
		for _, s := range srvs {
			s.Close()
		}
	}()

	responses, err := council.Roundtable(context.Background(), "q", participants)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, name := range names {
		if responses[i].Name != name {
			t.Errorf("position %d: expected %s, got %s", i, name, responses[i].Name)
		}
	}
}

func TestSynthesize_callsModerator(t *testing.T) {
	srv1, p1 := newParticipant("a", okHandler("view A"))
	srv2, p2 := newParticipant("b", okHandler("view B"))
	defer srv1.Close()
	defer srv2.Close()

	var moderatorSaw string
	modSrv, modP := newParticipant("mod", func(w http.ResponseWriter, r *http.Request) {
		var req chat.CreateRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) > 0 {
			if s, ok := req.Messages[len(req.Messages)-1].Content.(string); ok {
				moderatorSaw = s
			}
		}
		json.NewEncoder(w).Encode(chat.Completion{
			Choices: []chat.Choice{
				{Message: chat.Message{Role: "assistant", Content: "synthesized"}},
			},
		})
	})
	defer modSrv.Close()

	result, err := council.Synthesize(context.Background(), "what is the answer?", []council.Participant{p1, p2}, modP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "synthesized" {
		t.Errorf("unexpected result: %s", result)
	}
	if !strings.Contains(moderatorSaw, "view A") || !strings.Contains(moderatorSaw, "view B") {
		t.Errorf("moderator prompt should contain participant outputs, got: %s", moderatorSaw)
	}
}
