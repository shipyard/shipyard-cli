package errors

import (
	"fmt"
	"net/http"
	"strings"
)

// MCPError represents an MCP-specific error with context and suggestions
type MCPError struct {
	Operation  string // The operation that failed (e.g., "get_environment")
	Message    string // Human-readable error message
	Suggestion string // Optional suggestion for the user
	Cause      error  // The underlying error
	StatusCode int    // HTTP status code if applicable
}

func (e *MCPError) Error() string {
	msg := fmt.Sprintf("%s failed: %s", e.Operation, e.Message)
	if e.Suggestion != "" {
		msg += fmt.Sprintf(". %s", e.Suggestion)
	}
	return msg
}

func (e *MCPError) Unwrap() error {
	return e.Cause
}

// NewMCPError creates a new MCP error with context
func NewMCPError(operation, message string, cause error) *MCPError {
	return &MCPError{
		Operation: operation,
		Message:   message,
		Cause:     cause,
	}
}

// WithSuggestion adds a suggestion to the error
func (e *MCPError) WithSuggestion(suggestion string) *MCPError {
	e.Suggestion = suggestion
	return e
}

// WithStatusCode adds an HTTP status code to the error
func (e *MCPError) WithStatusCode(code int) *MCPError {
	e.StatusCode = code
	return e
}

// Common error creators for different types of failures

// ValidationError creates an error for parameter validation failures
func ValidationError(operation, parameter, issue string) *MCPError {
	message := fmt.Sprintf("invalid %s: %s", parameter, issue)
	suggestion := fmt.Sprintf("Please check the %s parameter and try again", parameter)
	return &MCPError{
		Operation:  operation,
		Message:    message,
		Suggestion: suggestion,
	}
}

// NotFoundError creates an error for when a resource is not found
func NotFoundError(operation, resourceType, resourceID string) *MCPError {
	message := fmt.Sprintf("%s '%s' not found", resourceType, resourceID)
	var suggestion string

	switch resourceType {
	case "environment":
		suggestion = "Verify the environment ID exists and you have access to it. Use 'get_environments' to list available environments"
	case "service":
		suggestion = "Verify the service name exists in the environment. Use 'get_services' to list available services"
	case "organization":
		suggestion = "Verify the organization name exists and you have access to it. Use 'get_orgs' to list available organizations"
	default:
		suggestion = fmt.Sprintf("Verify the %s exists and you have access to it", resourceType)
	}

	return &MCPError{
		Operation:  operation,
		Message:    message,
		Suggestion: suggestion,
		StatusCode: http.StatusNotFound,
	}
}

// PermissionError creates an error for permission/access issues
func PermissionError(operation, resourceType, resourceID string) *MCPError {
	message := fmt.Sprintf("access denied to %s '%s'", resourceType, resourceID)
	suggestion := "Verify you have the necessary permissions for this resource. Contact your administrator if needed"
	return &MCPError{
		Operation:  operation,
		Message:    message,
		Suggestion: suggestion,
		StatusCode: http.StatusForbidden,
	}
}

// ConflictError creates an error for state conflicts
func ConflictError(operation, action, resourceType, resourceID, currentState string) *MCPError {
	message := fmt.Sprintf("cannot %s %s '%s' (currently %s)", action, resourceType, resourceID, currentState)
	var suggestion string

	switch action {
	case "start", "restart":
		if currentState == "running" {
			suggestion = "Environment is already running. Use 'stop_environment' first if you need to restart it"
		} else if currentState == "building" {
			suggestion = "Environment is currently building. Wait for build to complete or use 'cancel_environment' to stop it"
		}
	case "stop":
		if currentState == "stopped" {
			suggestion = "Environment is already stopped"
		} else if currentState == "building" {
			suggestion = "Environment is building. Use 'cancel_environment' to stop the build process"
		}
	case "rebuild":
		if currentState == "building" {
			suggestion = "Environment is already building. Use 'cancel_environment' to stop current build first"
		}
	}

	if suggestion == "" {
		suggestion = fmt.Sprintf("Check the current state with 'get_environment' and try an appropriate action")
	}

	return &MCPError{
		Operation:  operation,
		Message:    message,
		Suggestion: suggestion,
		StatusCode: http.StatusConflict,
	}
}

// NetworkError creates an error for network/connectivity issues
func NetworkError(operation string, cause error) *MCPError {
	message := "network or connectivity issue"
	suggestion := "Check your internet connection and Shipyard service status. Retry the operation"
	return &MCPError{
		Operation:  operation,
		Message:    message,
		Suggestion: suggestion,
		Cause:      cause,
	}
}

// ServerError creates an error for server-side issues
func ServerError(operation string, cause error) *MCPError {
	message := "internal server error"
	suggestion := "This appears to be a temporary server issue. Please try again in a few moments"
	return &MCPError{
		Operation:  operation,
		Message:    message,
		Suggestion: suggestion,
		Cause:      cause,
		StatusCode: http.StatusInternalServerError,
	}
}

// ParseHTTPError converts HTTP errors into user-friendly MCP errors
func ParseHTTPError(operation string, err error, resourceID string) *MCPError {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	// Use a more descriptive default if no resourceID provided
	if resourceID == "" {
		resourceID = "requested"
	}

	// Check for common HTTP status patterns
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
		// Try to extract resource type from operation
		resourceType := "resource"
		if strings.Contains(operation, "environment") {
			resourceType = "environment"
		} else if strings.Contains(operation, "service") {
			resourceType = "service"
		} else if strings.Contains(operation, "org") {
			resourceType = "organization"
		}
		return NotFoundError(operation, resourceType, resourceID)
	}

	if strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") {
		message := "authentication required or invalid"
		suggestion := "Please authenticate using 'shipyard login' or verify your API token"
		return &MCPError{
			Operation:  operation,
			Message:    message,
			Suggestion: suggestion,
			StatusCode: http.StatusUnauthorized,
		}
	}

	if strings.Contains(errStr, "403") || strings.Contains(errStr, "forbidden") {
		return PermissionError(operation, "resource", resourceID)
	}

	if strings.Contains(errStr, "409") || strings.Contains(errStr, "conflict") {
		return ConflictError(operation, "perform action on", "resource", resourceID, "current state")
	}

	if strings.Contains(errStr, "500") || strings.Contains(errStr, "internal server error") {
		return ServerError(operation, err)
	}

	// Check for network issues
	if strings.Contains(errStr, "connection") || strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "network") || strings.Contains(errStr, "dial") {
		return NetworkError(operation, err)
	}

	// Default case - wrap the original error with operation context
	return NewMCPError(operation, err.Error(), err).WithSuggestion("Please check the operation parameters and try again")
}

// ToJSONRPCError converts an MCPError to appropriate JSON-RPC error code
func (e *MCPError) ToJSONRPCCode() int {
	switch e.StatusCode {
	case http.StatusBadRequest:
		return -32602 // Invalid params
	case http.StatusUnauthorized:
		return -32001 // Authentication error
	case http.StatusForbidden:
		return -32002 // Permission error
	case http.StatusNotFound:
		return -32003 // Resource not found
	case http.StatusConflict:
		return -32004 // State conflict
	case http.StatusInternalServerError:
		return -32603 // Internal error
	default:
		return -32000 // Server error (generic)
	}
}
