package environment

import (
	"strings"
)

// BusinessError represents a structured business logic error
type BusinessError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Context map[string]interface{} `json:"context,omitempty"`
}

func (e *BusinessError) Error() string {
	return e.Message
}

// Error codes for common business logic scenarios
const (
	ErrorCodeEnvironmentNotPaused = "ENVIRONMENT_NOT_PAUSED"
	ErrorCodeNoBuildFound         = "NO_BUILD_FOUND"
	ErrorCodeEnvironmentNotFound  = "ENVIRONMENT_NOT_FOUND"
	ErrorCodeInvalidToken         = "INVALID_TOKEN"
	ErrorCodeOrgNotFound          = "ORG_NOT_FOUND"
	ErrorCodeUnknown              = "UNKNOWN_ERROR"
)

// ParseAPIError analyzes an API error and converts it to a BusinessError if possible
func ParseAPIError(err error, context map[string]interface{}) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// Classify known error patterns
	switch {
	case strings.Contains(errMsg, "this environment is not paused"):
		return &BusinessError{
			Code:    ErrorCodeEnvironmentNotPaused,
			Message: errMsg,
			Context: context,
		}
	case strings.Contains(errMsg, "no builds for this environment"):
		return &BusinessError{
			Code:    ErrorCodeNoBuildFound,
			Message: errMsg,
			Context: context,
		}
	case strings.Contains(errMsg, "environment not found"):
		return &BusinessError{
			Code:    ErrorCodeEnvironmentNotFound,
			Message: errMsg,
			Context: context,
		}
	case strings.Contains(errMsg, "invalid token"):
		return &BusinessError{
			Code:    ErrorCodeInvalidToken,
			Message: errMsg,
			Context: context,
		}
	case strings.Contains(errMsg, "user org not found"):
		return &BusinessError{
			Code:    ErrorCodeOrgNotFound,
			Message: errMsg,
			Context: context,
		}
	default:
		// For unrecognized errors, still wrap them but mark as unknown
		return &BusinessError{
			Code:    ErrorCodeUnknown,
			Message: errMsg,
			Context: context,
		}
	}
}

// IsBusinessError checks if an error is a BusinessError
func IsBusinessError(err error) bool {
	_, ok := err.(*BusinessError)
	return ok
}

// GetErrorCode extracts the error code from a BusinessError, or returns UNKNOWN for other errors
func GetErrorCode(err error) string {
	if bizErr, ok := err.(*BusinessError); ok {
		return bizErr.Code
	}
	return ErrorCodeUnknown
}

// GetErrorContext extracts the context from a BusinessError
func GetErrorContext(err error) map[string]interface{} {
	if bizErr, ok := err.(*BusinessError); ok {
		return bizErr.Context
	}
	return nil
}
