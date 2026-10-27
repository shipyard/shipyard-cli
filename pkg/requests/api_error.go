package requests

import (
	"encoding/json"
	"errors"
)

// APIError is the error Do returns for a non-2xx response. Error() is the same
// message callers always got; StatusCode and Body let a caller read structured
// fields from the response, such as core's status and next_step.
type APIError struct {
	StatusCode int
	// Body is the raw response body, possibly empty.
	Body    []byte
	message string
}

func (e *APIError) Error() string {
	return e.message
}

// AsAPIError reports whether err is, or wraps, an *APIError.
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}

// JSONObject decodes Body as a JSON object. It returns nil when Body is empty
// or not an object.
func (e *APIError) JSONObject() map[string]any {
	var obj map[string]any
	if err := json.Unmarshal(e.Body, &obj); err != nil {
		return nil
	}
	return obj
}
