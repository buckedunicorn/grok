package grok

import "github.com/buckedunicorn/grok/internal/apierr"

// APIError is returned when the xAI API responds with a non-2xx status code.
// Use errors.As to extract it from any error returned by this package:
//
//	var apiErr *grok.APIError
//	if errors.As(err, &apiErr) {
//	    fmt.Println(apiErr.StatusCode, apiErr.Message)
//	}
type APIError = apierr.APIError
