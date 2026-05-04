package chat_test

import (
	"errors"
	"testing"

	"github.com/buckedunicorn/grok/chat"
)

func completionWith(content any) *chat.Completion {
	return &chat.Completion{
		Choices: []chat.Choice{{Message: chat.Message{Role: "assistant", Content: content}}},
	}
}

func TestParseList_defaultNewline(t *testing.T) {
	got, err := chat.ParseList(completionWith("apple\nbanana\ncherry\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "apple" || got[2] != "cherry" {
		t.Errorf("got %+v", got)
	}
}

func TestParseList_customSeparatorAndTrim(t *testing.T) {
	got, err := chat.ParseList(completionWith(" alpha , beta ,, gamma "), ",")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1] != "beta" {
		t.Errorf("got %+v", got)
	}
}

func TestParseList_emptyCompletion(t *testing.T) {
	_, err := chat.ParseList(&chat.Completion{}, ",")
	if !errors.Is(err, chat.ErrEmptyCompletion) {
		t.Errorf("err = %v, want ErrEmptyCompletion", err)
	}
}

func TestParseRegex_firstGroup(t *testing.T) {
	got, err := chat.ParseRegex(completionWith("answer: 42 (out of 100)"), `answer:\s*(\d+)`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "42" {
		t.Errorf("got %q", got)
	}
}

func TestParseRegex_wholeMatchWhenNoGroup(t *testing.T) {
	got, err := chat.ParseRegex(completionWith("file_42.txt"), `file_\d+\.txt`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "file_42.txt" {
		t.Errorf("got %q", got)
	}
}

func TestParseRegex_noMatch(t *testing.T) {
	_, err := chat.ParseRegex(completionWith("nothing here"), `xyz`)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseRegex_invalidPattern(t *testing.T) {
	_, err := chat.ParseRegex(completionWith("anything"), `[unclosed`)
	if err == nil {
		t.Fatal("expected compile error")
	}
}

func TestParse_nonStringContent(t *testing.T) {
	_, err := chat.ParseList(completionWith(123), ",")
	if err == nil {
		t.Fatal("expected error for non-string content")
	}
}
