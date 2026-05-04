// Package apierr defines the shared API error type for the grok SDK.
// It lives in internal/ to avoid import cycles; the root package re-exports it
// as a type alias so callers use *grok.APIError.
package apierr

import "fmt"

// APIError is returned when the xAI API responds with a non-2xx status.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Raw        []byte
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("grok: HTTP %d %s: %s", e.StatusCode, e.Code, e.Message)
	}
	if e.Message != "" {
		return fmt.Sprintf("grok: HTTP %d: %s", e.StatusCode, e.Message)
	}
	// Fallback: show raw body so callers can diagnose unexpected error formats.
	if len(e.Raw) > 0 {
		return fmt.Sprintf("grok: HTTP %d: %s", e.StatusCode, e.Raw)
	}
	return fmt.Sprintf("grok: HTTP %d", e.StatusCode)
}
