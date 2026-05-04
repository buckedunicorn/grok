package prompt_test

import (
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/prompt"
)

func TestTemplate_Render_basic(t *testing.T) {
	tmpl := prompt.MustNew("Hello, {name}!")
	got, err := tmpl.Render(map[string]any{"name": "world"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello, world!" {
		t.Errorf("got %q", got)
	}
}

func TestTemplate_Render_typeCoercion(t *testing.T) {
	tmpl := prompt.MustNew("count={n} ratio={r} flag={b}")
	got, err := tmpl.Render(map[string]any{"n": 42, "r": 0.5, "b": true})
	if err != nil {
		t.Fatal(err)
	}
	if got != "count=42 ratio=0.5 flag=true" {
		t.Errorf("got %q", got)
	}
}

func TestTemplate_Render_repeatedVar(t *testing.T) {
	tmpl := prompt.MustNew("{x} and {x} again")
	got, _ := tmpl.Render(map[string]any{"x": "ok"})
	if got != "ok and ok again" {
		t.Errorf("got %q", got)
	}
}

func TestTemplate_Render_whitespaceInsideBraces(t *testing.T) {
	tmpl := prompt.MustNew("a={  name  } b={name}")
	got, _ := tmpl.Render(map[string]any{"name": "v"})
	if got != "a=v b=v" {
		t.Errorf("got %q", got)
	}
}

func TestTemplate_Render_missingVar(t *testing.T) {
	tmpl := prompt.MustNew("{a} {b} {c}")
	_, err := tmpl.Render(map[string]any{"a": "1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "b") || !strings.Contains(err.Error(), "c") {
		t.Errorf("err should list missing vars, got: %v", err)
	}
}

func TestTemplate_Vars(t *testing.T) {
	tmpl := prompt.MustNew("{first} {second} {first}")
	got := tmpl.Vars()
	want := []string{"first", "second"}
	if len(got) != len(want) {
		t.Fatalf("vars = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("vars[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTemplate_Partial(t *testing.T) {
	tmpl := prompt.MustNew("greeting: {greeting}, name: {name}")
	bound := tmpl.Partial(map[string]any{"greeting": "hello"})
	if vars := bound.Vars(); len(vars) != 1 || vars[0] != "name" {
		t.Errorf("after Partial, Vars = %v, want [name]", vars)
	}
	got, err := bound.Render(map[string]any{"name": "world"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "greeting: hello, name: world" {
		t.Errorf("got %q", got)
	}
}

func TestTemplate_NonPlaceholderBraces(t *testing.T) {
	// Stray braces with no valid identifier inside should pass through.
	tmpl := prompt.MustNew("set := {1, 2, 3}; var = {x}")
	got, err := tmpl.Render(map[string]any{"x": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "set := {1, 2, 3}; var = ok" {
		t.Errorf("got %q", got)
	}
}

func TestChatTemplate_Render(t *testing.T) {
	ct := &prompt.ChatTemplate{
		System: "You are a {persona} assistant.",
		User:   "{question}",
	}
	msgs, err := ct.Render(map[string]any{
		"persona":  "concise",
		"question": "What is Go?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "You are a concise assistant." {
		t.Errorf("system = %+v", msgs[0])
	}
	if msgs[1].Role != "user" || msgs[1].Content != "What is Go?" {
		t.Errorf("user = %+v", msgs[1])
	}
}

func TestChatTemplate_emptySystemOmitted(t *testing.T) {
	ct := &prompt.ChatTemplate{User: "hi {name}"}
	msgs, err := ct.Render(map[string]any{"name": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Errorf("expected single user message, got %+v", msgs)
	}
}

func TestChatTemplate_missingVarErrorsCleanly(t *testing.T) {
	ct := &prompt.ChatTemplate{System: "hi {missing}", User: "u"}
	_, err := ct.Render(map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
}
