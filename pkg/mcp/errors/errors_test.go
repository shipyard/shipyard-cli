package errors

import (
	"net/http"
	"strings"
	"testing"
)

func TestMCPError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *MCPError
		expected string
	}{
		{
			name: "basic error",
			err: &MCPError{
				Operation: "get_environment",
				Message:   "environment not found",
			},
			expected: "get_environment failed: environment not found",
		},
		{
			name: "error with suggestion",
			err: &MCPError{
				Operation:  "get_environment",
				Message:    "environment not found",
				Suggestion: "Verify the environment ID exists",
			},
			expected: "get_environment failed: environment not found. Verify the environment ID exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("MCPError.Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	err := ValidationError("get_environment", "environment_id", "too short")

	if !strings.Contains(err.Error(), "get_environment failed") {
		t.Errorf("Expected error to contain operation name")
	}

	if !strings.Contains(err.Error(), "invalid environment_id") {
		t.Errorf("Expected error to contain parameter name")
	}

	if !strings.Contains(err.Error(), "too short") {
		t.Errorf("Expected error to contain issue description")
	}

	if !strings.Contains(err.Error(), "Please check the environment_id parameter") {
		t.Errorf("Expected error to contain suggestion")
	}
}

func TestNotFoundError(t *testing.T) {
	err := NotFoundError("get_environment", "environment", "env-123")

	if !strings.Contains(err.Error(), "environment 'env-123' not found") {
		t.Errorf("Expected error to contain resource info")
	}

	if !strings.Contains(err.Error(), "get_environments") {
		t.Errorf("Expected error to suggest listing environments")
	}

	if err.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status code 404, got %d", err.StatusCode)
	}
}

func TestConflictError(t *testing.T) {
	err := ConflictError("restart_environment", "restart", "environment", "env-123", "running")

	if !strings.Contains(err.Error(), "cannot restart environment 'env-123' (currently running)") {
		t.Errorf("Expected error to describe conflict")
	}

	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("Expected error to provide context-appropriate suggestion")
	}

	if err.StatusCode != http.StatusConflict {
		t.Errorf("Expected status code 409, got %d", err.StatusCode)
	}
}

func TestParseHTTPError(t *testing.T) {
	tests := []struct {
		name         string
		operation    string
		inputError   error
		resourceID   string
		expectedCode int
		expectedMsg  string
	}{
		{
			name:         "404 error with resource ID",
			operation:    "get_environment",
			inputError:   &testHTTPError{msg: "404 not found"},
			resourceID:   "env-123",
			expectedCode: http.StatusNotFound,
			expectedMsg:  "environment 'env-123' not found",
		},
		{
			name:         "404 error without resource ID",
			operation:    "get_environment",
			inputError:   &testHTTPError{msg: "404 not found"},
			resourceID:   "",
			expectedCode: http.StatusNotFound,
			expectedMsg:  "environment 'requested' not found",
		},
		{
			name:         "401 error",
			operation:    "get_environment",
			inputError:   &testHTTPError{msg: "401 unauthorized"},
			resourceID:   "env-456",
			expectedCode: http.StatusUnauthorized,
			expectedMsg:  "authentication",
		},
		{
			name:         "403 error with resource ID",
			operation:    "get_environment",
			inputError:   &testHTTPError{msg: "403 forbidden"},
			resourceID:   "env-789",
			expectedCode: http.StatusForbidden,
			expectedMsg:  "access denied to resource 'env-789'",
		},
		{
			name:         "network error",
			operation:    "get_environment",
			inputError:   &testHTTPError{msg: "connection timeout"},
			resourceID:   "env-999",
			expectedCode: 0,
			expectedMsg:  "network",
		},
		{
			name:         "service operation 404",
			operation:    "get_services",
			inputError:   &testHTTPError{msg: "404 not found"},
			resourceID:   "env-service-123",
			expectedCode: http.StatusNotFound,
			expectedMsg:  "service 'env-service-123' not found",
		},
		{
			name:         "org operation 404",
			operation:    "get_orgs",
			inputError:   &testHTTPError{msg: "404 not found"},
			resourceID:   "my-org",
			expectedCode: http.StatusNotFound,
			expectedMsg:  "organization 'my-org' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpErr := ParseHTTPError(tt.operation, tt.inputError, tt.resourceID)

			if tt.expectedCode != 0 && mcpErr.StatusCode != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedCode, mcpErr.StatusCode)
			}

			if !strings.Contains(strings.ToLower(mcpErr.Error()), strings.ToLower(tt.expectedMsg)) {
				t.Errorf("Expected error message to contain '%s', got: %s", tt.expectedMsg, mcpErr.Error())
			}
		})
	}
}

func TestToJSONRPCCode(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		expectedCode int
	}{
		{"bad request", http.StatusBadRequest, -32602},
		{"unauthorized", http.StatusUnauthorized, -32001},
		{"forbidden", http.StatusForbidden, -32002},
		{"not found", http.StatusNotFound, -32003},
		{"conflict", http.StatusConflict, -32004},
		{"internal error", http.StatusInternalServerError, -32603},
		{"default", http.StatusTeapot, -32000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &MCPError{StatusCode: tt.statusCode}
			if got := err.ToJSONRPCCode(); got != tt.expectedCode {
				t.Errorf("ToJSONRPCCode() = %v, want %v", got, tt.expectedCode)
			}
		})
	}
}

// Test helper
type testHTTPError struct {
	msg string
}

func (e *testHTTPError) Error() string {
	return e.msg
}
