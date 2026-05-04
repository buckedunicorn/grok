package apierr_test

import (
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/internal/apierr"
)

func TestAPIError_Error(t *testing.T) {
	cases := []struct {
		name string
		err  *apierr.APIError
		want string
	}{
		{
			name: "code and message",
			err:  &apierr.APIError{StatusCode: 429, Code: "rate_limited", Message: "too many requests"},
			want: "grok: HTTP 429 rate_limited: too many requests",
		},
		{
			name: "message only",
			err:  &apierr.APIError{StatusCode: 401, Message: "unauthorized"},
			want: "grok: HTTP 401: unauthorized",
		},
		{
			name: "raw only",
			err:  &apierr.APIError{StatusCode: 400, Raw: []byte(`{"unexpected":"format"}`)},
			want: `grok: HTTP 400: {"unexpected":"format"}`,
		},
		{
			name: "status only",
			err:  &apierr.APIError{StatusCode: 500},
			want: "grok: HTTP 500",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.err.Error(); got != c.want {
				t.Errorf("Error() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestAPIError_Error_containsStatus(t *testing.T) {
	err := &apierr.APIError{StatusCode: 503}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("Error() = %q, should contain status code", err.Error())
	}
}
