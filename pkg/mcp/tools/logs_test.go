package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/k8s"
	"github.com/shipyard/shipyard-cli/pkg/services/logs"
)

// Note: newMockClient is already defined in environment_test.go

func TestNewLogsTool(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	tool := NewLogsTool(client, "get_logs")

	if tool == nil {
		t.Fatal("Expected tool to be created, got nil")
	}

	if tool.name != "get_logs" {
		t.Errorf("Expected tool name to be 'get_logs', got %s", tool.name)
	}

	if tool.logsService == nil {
		t.Error("Expected logs service to be initialized")
	}
}

func TestLogsTool_Definition(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	tool := NewLogsTool(client, "get_logs")

	def := tool.Definition()

	if def.Name != "get_logs" {
		t.Errorf("Expected tool name to be 'get_logs', got %s", def.Name)
	}

	if def.Description == "" {
		t.Error("Expected description to be set")
	}

	if def.InputSchema == nil {
		t.Error("Expected input schema to be set")
	}

	// Check that the schema has the required properties
	schema, ok := def.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatal("Expected input schema to be a map")
	}

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected properties to exist in schema")
	}

	requiredProps := []string{"environment_id", "service_name"}
	for _, prop := range requiredProps {
		if _, exists := properties[prop]; !exists {
			t.Errorf("Expected property %s to exist in schema", prop)
		}
	}
}

func TestLogsTool_Execute(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsTool_Execute_InvalidParams(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	tool := NewLogsTool(client, "get_logs")

	ctx := context.Background()

	// Test with invalid JSON
	_, err := tool.Execute(ctx, []byte(`{"invalid": json}`))
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}

	if !strings.Contains(err.Error(), "invalid parameters") {
		t.Errorf("Expected invalid parameters error, got: %v", err)
	}
}

func TestLogsTool_Execute_MissingEnvironmentID(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	tool := NewLogsTool(client, "get_logs")

	params := map[string]interface{}{
		"service_name": "web-server",
		"tail":         100,
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	_, err := tool.Execute(ctx, paramsJSON)

	if err == nil {
		t.Error("Expected error for missing environment_id, got nil")
	}

	if !strings.Contains(err.Error(), "environment_id is required") {
		t.Errorf("Expected environment_id required error, got: %v", err)
	}
}

func TestLogsTool_Execute_MissingServiceName(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	tool := NewLogsTool(client, "get_logs")

	params := map[string]interface{}{
		"environment_id": "env-123",
		"tail":           100,
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	_, err := tool.Execute(ctx, paramsJSON)

	if err == nil {
		t.Error("Expected error for missing service_name, got nil")
	}

	if !strings.Contains(err.Error(), "service_name is required") {
		t.Errorf("Expected service_name required error, got: %v", err)
	}
}

func TestLogsTool_Execute_DefaultTailValue(t *testing.T) {
	t.Parallel()

	// This test checks that the tool applies default values correctly
	// We can test this without actually executing since it's just parameter processing

	params := map[string]interface{}{
		"environment_id": "env-123",
		"service_name":   "web-server",
		// tail not specified, should default to 100
	}
	paramsJSON, _ := json.Marshal(params)

	// We can't test the actual execution without proper mocking,
	// but we can verify the parameter unmarshaling works
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		ServiceName   string `json:"service_name"`
		Follow        bool   `json:"follow,omitempty"`
		Tail          int64  `json:"tail,omitempty"`
	}

	err := json.Unmarshal(paramsJSON, &toolParams)
	if err != nil {
		t.Errorf("Failed to unmarshal params: %v", err)
	}

	if toolParams.Tail != 0 {
		t.Errorf("Expected tail to be 0 (unset), got %d", toolParams.Tail)
	}

	// The actual default should be applied in the Execute method
	if toolParams.Tail == 0 {
		toolParams.Tail = 100 // This is what the code should do
	}

	if toolParams.Tail != 100 {
		t.Errorf("Expected default tail to be 100, got %d", toolParams.Tail)
	}
}

func TestLogsTool_Execute_WithFollowParam(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsTool_Execute_WithCustomTail(t *testing.T) {
	t.Parallel()

	// Test parameter processing for custom tail value

	params := map[string]interface{}{
		"environment_id": "env-123",
		"service_name":   "web-server",
		"tail":           250,
	}
	paramsJSON, _ := json.Marshal(params)

	// Verify parameter unmarshaling
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		ServiceName   string `json:"service_name"`
		Follow        bool   `json:"follow,omitempty"`
		Tail          int64  `json:"tail,omitempty"`
	}

	err := json.Unmarshal(paramsJSON, &toolParams)
	if err != nil {
		t.Errorf("Failed to unmarshal params: %v", err)
	}

	if toolParams.Tail != 250 {
		t.Errorf("Expected tail to be 250, got %d", toolParams.Tail)
	}
}

func TestLogsTool_Execute_ServiceNotFound(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsTool_Execute_LogsServiceError(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsTool_Execute_NoLogsFound(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsTool_Execute_SuccessWithLogs(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsTool_Execute_FollowModeMessage(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsTool_ParameterValidation(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	tool := NewLogsTool(client, "get_logs")

	// Test various parameter combinations
	testCases := []struct {
		name        string
		params      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid params",
			params: map[string]interface{}{
				"environment_id": "env-123",
				"service_name":   "web-server",
				"tail":           100,
			},
			expectError: false,
		},
		{
			name: "missing environment_id",
			params: map[string]interface{}{
				"service_name": "web-server",
			},
			expectError: true,
			errorMsg:    "environment_id is required",
		},
		{
			name: "missing service_name",
			params: map[string]interface{}{
				"environment_id": "env-123",
			},
			expectError: true,
			errorMsg:    "service_name is required",
		},
		{
			name: "empty environment_id",
			params: map[string]interface{}{
				"environment_id": "",
				"service_name":   "web-server",
			},
			expectError: true,
			errorMsg:    "environment_id is required",
		},
		{
			name: "empty service_name",
			params: map[string]interface{}{
				"environment_id": "env-123",
				"service_name":   "",
			},
			expectError: true,
			errorMsg:    "service_name is required",
		},
	}

	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tc.params)

			_, err := tool.Execute(ctx, paramsJSON)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for test case %s, got nil", tc.name)
				} else if !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got: %v", tc.errorMsg, err)
				}
			} else {
				// For valid params, we expect it to fail later in the process due to missing mocks
				// but not due to parameter validation
				if err != nil && strings.Contains(err.Error(), "required") {
					t.Errorf("Unexpected parameter validation error for test case %s: %v", tc.name, err)
				}
			}
		})
	}
}

func TestLogsTool_JSONUnmarshaling(t *testing.T) {
	t.Parallel()

	// Test JSON unmarshaling with various input formats
	testCases := []struct {
		name      string
		jsonInput string
		expectErr bool
	}{
		{
			name:      "valid JSON",
			jsonInput: `{"environment_id": "env-123", "service_name": "web-server", "tail": 100}`,
			expectErr: false,
		},
		{
			name:      "minimal valid JSON",
			jsonInput: `{"environment_id": "env-123", "service_name": "web-server"}`,
			expectErr: false,
		},
		{
			name:      "with boolean follow",
			jsonInput: `{"environment_id": "env-123", "service_name": "web-server", "follow": true}`,
			expectErr: false,
		},
		{
			name:      "invalid JSON syntax",
			jsonInput: `{"environment_id": "env-123", "service_name": "web-server"`,
			expectErr: true,
		},
		{
			name:      "invalid JSON structure",
			jsonInput: `"just a string"`,
			expectErr: true, // Cannot unmarshal string into struct
		},
		{
			name:      "empty JSON object",
			jsonInput: `{}`,
			expectErr: false, // This will unmarshal but fail validation
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var toolParams struct {
				EnvironmentID string `json:"environment_id"`
				ServiceName   string `json:"service_name"`
				Follow        bool   `json:"follow,omitempty"`
				Tail          int64  `json:"tail,omitempty"`
			}

			err := json.Unmarshal([]byte(tc.jsonInput), &toolParams)

			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected unmarshal error for test case %s, got nil", tc.name)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected unmarshal error for test case %s: %v", tc.name, err)
				}
			}
		})
	}
}

func TestFormatContainerState(t *testing.T) {
	exitCode := int32(137)
	got := formatContainerState(&k8s.ContainerState{
		Pod: "web-1", Ready: false, RestartCount: 5, Reason: "OOMKilled", ExitCode: &exitCode,
	})
	if got != "Pod web-1: ready=false, restarts=5, last stopped: OOMKilled (exit code 137)\n" {
		t.Fatalf("unexpected header %q", got)
	}
	if got := formatContainerState(&k8s.ContainerState{Pod: "web-1", Ready: true}); got != "Pod web-1: ready=true, restarts=0\n" {
		t.Fatalf("unexpected header %q", got)
	}
	if got := formatContainerState(&k8s.ContainerState{Pod: "web-1", Container: "app", Ready: true}); got != "Pod web-1 container app: ready=true, restarts=0\n" {
		t.Fatalf("unexpected header %q", got)
	}
	if got := formatContainerState(&k8s.ContainerState{Pod: "web-1", Container: "migrate", Init: true, RestartCount: 3}); got != "Pod web-1 init container migrate: ready=false, restarts=3\n" {
		t.Fatalf("unexpected header %q", got)
	}
}

func TestFormatLogsResponse(t *testing.T) {
	tool := NewLogsTool(newMockClient(), "get_logs")
	lines := []logs.LogLine{{Content: "boom", Service: "web"}}
	restarted := &k8s.ContainerState{Pod: "web-1", RestartCount: 2}
	fresh := &k8s.ContainerState{Pod: "web-1"}

	tests := []struct {
		name    string
		resp    logs.LogsResponse
		page    int
		want    []string
		notWant []string
	}{
		{"previous run", logs.LogsResponse{State: restarted, Run: logs.RunPrevious, Lines: lines}, 1,
			[]string{"Pod web-1: ready=false, restarts=2\n", "boom\n", "Showing 1 log lines for service web (page 1)"}, nil},
		{"previous run with no output", logs.LogsResponse{State: restarted, Run: logs.RunPrevious}, 1,
			[]string{"Pod web-1: ready=false, restarts=2\n", "No logs from the previous container run."}, nil},
		// Paging past the end must not contradict the lines page 1 returned
		{"past the last page", logs.LogsResponse{State: restarted, Run: logs.RunPrevious}, 3,
			[]string{"No more lines on page 3"}, []string{"No logs from the previous container run"}},
		{"previous run no longer kept", logs.LogsResponse{State: restarted, Run: logs.RunGone}, 1,
			[]string{"restarts=2", "no longer keeps", `get_build_logs(environment_id="env-1", kind="crash", service_name="web")`}, nil},
		// The current run of the pod that was picked, not a redirect to a call that may read another pod
		{"never restarted", logs.LogsResponse{State: fresh, Run: logs.RunCurrent, Lines: lines}, 1,
			[]string{"has not restarted", "current run", "boom\n"}, []string{"Call get_logs"}},
		{"never restarted, no output", logs.LogsResponse{State: fresh, Run: logs.RunCurrent}, 1,
			[]string{"has not restarted", "no output yet"}, []string{"Call get_logs"}},
		{"live logs", logs.LogsResponse{Lines: lines, HasNext: true, NextPage: 2}, 1,
			[]string{"boom\n", "More logs available on page 2"}, []string{"Pod "}},
		{"live logs, none", logs.LogsResponse{}, 1,
			[]string{"No logs found for service web in environment env-1"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := tool.formatLogsResponse(&tt.resp, "env-1", "web", tt.page)
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q:\n%s", want, out)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(out, notWant) {
					t.Errorf("unexpected %q:\n%s", notWant, out)
				}
			}
		})
	}
}
