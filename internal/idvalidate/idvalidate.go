// Package idvalidate is a tiny shared helper for client-side validation
// of opaque IDs that get concatenated into request paths.
//
// All API surfaces that interpolate a caller-supplied ID into the URL
// (files, videos, responses, chat.GetDeferred, models.Get*, voice.GetVoice,
// batches.* by batchID) call OpaqueID before passing the value through
// url.PathEscape. The check rejects path separators, control characters,
// and ".." so a caller-controlled value cannot reroute the request to a
// different endpoint or inject query parameters.
package idvalidate

import (
	"fmt"
	"strings"
)

// OpaqueID returns nil if id is safe to interpolate into a path
// component. The pkg name is used to namespace the error message so
// callers see "grok/files: fileID..." rather than a generic message.
func OpaqueID(pkg, field, id string) error {
	if id == "" {
		return fmt.Errorf("%s: %s is empty", pkg, field)
	}
	for i, r := range id {
		if r == '/' || r == '\\' || r == '?' || r == '#' || r == '%' || r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s: %s contains invalid byte at offset %d: %q", pkg, field, i, id)
		}
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("%s: %s must not contain path traversal: %q", pkg, field, id)
	}
	return nil
}
