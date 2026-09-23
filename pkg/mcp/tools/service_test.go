package tools

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/k8s"
	mcperrors "github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/types"
)

func TestServiceTool_Definition(t *testing.T) {
	tests := []struct {
		name         string
		toolName     string
		expectedName string
		expectedDesc string
	}{
		{
			name:         "get_services tool definition",
			toolName:     "get_services",
			expectedName: "get_services",
			expectedDesc: "List services in an environment",
		},
		{
			// NewServiceTool leaves exec disabled, and the description says so:
			// a model that reads it stops instead of calling the tool.
			name:         "exec_service tool definition, disabled",
			toolName:     "exec_service",
			expectedName: "exec_service",
			expectedDesc: "DISABLED on this server: calling this returns setup instructions, " +
				"not command output. Running commands in containers is off until the user sets " +
				"'mcp.allow_exec: true' in ~/.shipyard/config.yaml or SHIPYARD_MCP_ALLOW_EXEC=true " +
				"in this client's environment. Tell them that rather than calling this tool.",
		},
		{
			name:         "port_forward tool definition",
			toolName:     "port_forward",
			expectedName: "port_forward",
			expectedDesc: "Port forward services to local machine",
		},
	}

	// Create mock client for testing
	mockClient := client.New(&servicesMockRequester{}, func() string { return "test-org" })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewServiceTool(mockClient, tt.toolName)
			def := tool.Definition()

			if def.Name != tt.expectedName {
				t.Errorf("Expected name %s, got %s", tt.expectedName, def.Name)
			}
			if def.Description != tt.expectedDesc {
				t.Errorf("Expected description %s, got %s", tt.expectedDesc, def.Description)
			}
		})
	}
}

// Mock requester for testing services
type servicesMockRequester struct{}

func (m *servicesMockRequester) Do(method, uri string, contentType string, body interface{}) ([]byte, error) {
	// Return mock JSON response for environment with services
	if strings.Contains(uri, "environment/env-") && method == "GET" {
		return []byte(`{
			"data": {
				"id": "env-123",
				"attributes": {
					"name": "test-env",
					"url": "https://test.shipyard.build",
					"ready": true,
					"services": [
						{
							"name": "web",
							"ports": ["80", "443"],
							"url": "https://web.example.com"
						},
						{
							"name": "api",
							"ports": ["8080"],
							"url": "https://api.example.com"
						}
					]
				}
			}
		}`), nil
	}
	return nil, nil
}

func TestServiceTool_Execute_GetServices(t *testing.T) {
	mockClient := client.New(&servicesMockRequester{}, func() string { return "test-org" })
	tool := NewServiceTool(mockClient, "get_services")

	result, err := tool.Execute(context.Background(), []byte(`{"environment_id":"env-123"}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expectedSubstrings := []string{
		"\"service_count\": 2",
		"\"web\"",
		"\"api\"",
		"\"environment_id\": \"env-123\"",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(result, substr) {
			t.Errorf("Expected result to contain %q, got: %s", substr, result)
		}
	}
}

func TestServiceTool_Execute_ExecService(t *testing.T) {
	mockClient := client.New(&servicesMockRequester{}, func() string { return "test-org" })
	tool := NewServiceTool(mockClient, "exec_service")

	result, err := tool.Execute(context.Background(), []byte(`{"environment_id":"env-123","service_name":"web","command":["ls","-la"]}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Exec is off unless the operator enables it, so the default tool explains
	// how to turn it on and hands over the CLI command meanwhile.
	expectedSubstrings := []string{
		"Running commands in containers is disabled",
		"mcp.allow_exec",
		"SHIPYARD_MCP_ALLOW_EXEC",
		"shipyard exec --env env-123 --service web -- ls -la",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(result, substr) {
			t.Errorf("Expected result to contain %q, got: %s", substr, result)
		}
	}
}

func TestServiceTool_Execute_PortForward(t *testing.T) {
	mockClient := client.New(&servicesMockRequester{}, func() string { return "test-org" })
	tool := NewServiceTool(mockClient, "port_forward")

	result, err := tool.Execute(context.Background(), []byte(`{"environment_id":"env-123","service_name":"web","ports":["8080:80"]}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expectedSubstrings := []string{
		"Cannot start port forwarding via MCP",
		"shipyard port-forward --env env-123 --service web",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(result, substr) {
			t.Errorf("Expected result to contain %q, got: %s", substr, result)
		}
	}
}

func TestServiceTool_Execute_InvalidParams(t *testing.T) {
	mockClient := client.New(&servicesMockRequester{}, func() string { return "test-org" })
	tool := NewServiceTool(mockClient, "get_services")

	_, err := tool.Execute(context.Background(), []byte(`{"invalid": "params"}`))
	if err == nil {
		t.Fatal("Expected error for invalid environment_id")
	}

	if !strings.Contains(err.Error(), "invalid environment_id") {
		t.Errorf("Expected error to mention invalid environment_id, got: %v", err)
	}
}

// fakeExecutor stands in for a pod, so these tests need no cluster.
type fakeExecutor struct {
	out          k8s.ExecOutput
	err          error
	gotCommand   []string
	gotMaxBytes  int
	gotCtxCalled bool
}

func (f *fakeExecutor) ExecCapture(ctx context.Context, command []string, maxBytes int) (k8s.ExecOutput, error) {
	f.gotCommand = command
	f.gotMaxBytes = maxBytes
	f.gotCtxCalled = ctx != nil

	return f.out, f.err
}

// execToolWith builds an exec tool with exec enabled and the pod faked out.
func execToolWith(fake *fakeExecutor) *ServiceTool {
	mockClient := client.New(&servicesMockRequester{}, func() string { return "test-org" })
	tool := NewServiceToolWithExec(mockClient, "exec_service", true)
	tool.newExecutor = func(client.Client, string, *types.Service) (podExecutor, error) {
		return fake, nil
	}

	return tool
}

func TestServiceTool_ExecService_Enabled(t *testing.T) {
	fake := &fakeExecutor{out: k8s.ExecOutput{Stdout: "total 0\napp\n", Stderr: "", ExitCode: 0}}
	tool := execToolWith(fake)

	result, err := tool.Execute(context.Background(),
		[]byte(`{"environment_id":"env-123","service_name":"web","command":["ls","-la"]}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("expected JSON, got %q: %v", result, err)
	}

	if got["stdout"] != "total 0\napp\n" {
		t.Errorf("stdout = %q, want the command's output", got["stdout"])
	}
	if got["exit_code"].(float64) != 0 {
		t.Errorf("exit_code = %v, want 0", got["exit_code"])
	}
	if _, present := got["truncated"]; present {
		t.Error("truncated should be absent when nothing was cut")
	}

	// The command must reach the pod as given, and the cap must be applied.
	if strings.Join(fake.gotCommand, " ") != "ls -la" {
		t.Errorf("command reached the pod as %v", fake.gotCommand)
	}
	if fake.gotMaxBytes != execMaxOutputBytes {
		t.Errorf("maxBytes = %d, want %d", fake.gotMaxBytes, execMaxOutputBytes)
	}
	if !fake.gotCtxCalled {
		t.Error("expected a context to be passed so the timeout applies")
	}
}

// A command that exits non-zero ran successfully; its exit code is the answer,
// not an error that throws the output away.
func TestServiceTool_ExecService_NonZeroExitIsAResult(t *testing.T) {
	fake := &fakeExecutor{out: k8s.ExecOutput{Stdout: "", Stderr: "no such file\n", ExitCode: 2}}
	tool := execToolWith(fake)

	result, err := tool.Execute(context.Background(),
		[]byte(`{"environment_id":"env-123","service_name":"web","command":["cat","/nope"]}`))
	if err != nil {
		t.Fatalf("a non-zero exit must not be an error, got: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("expected JSON, got %q: %v", result, err)
	}

	if got["exit_code"].(float64) != 2 {
		t.Errorf("exit_code = %v, want 2", got["exit_code"])
	}
	if got["stderr"] != "no such file\n" {
		t.Errorf("stderr = %q, want the command's stderr", got["stderr"])
	}
}

// A timeout keeps whatever the command printed first, which is usually the part
// worth reading.
func TestServiceTool_ExecService_TimeoutKeepsPartialOutput(t *testing.T) {
	fake := &fakeExecutor{
		out: k8s.ExecOutput{Stdout: "starting migration\n"},
		err: context.DeadlineExceeded,
	}
	tool := execToolWith(fake)

	result, err := tool.Execute(context.Background(),
		[]byte(`{"environment_id":"env-123","service_name":"web","command":["sleep","600"]}`))
	if err != nil {
		t.Fatalf("a timeout should report partial output, not fail: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("expected JSON, got %q: %v", result, err)
	}

	if got["stdout"] != "starting migration\n" {
		t.Errorf("stdout = %q, want the output printed before the timeout", got["stdout"])
	}
	note, _ := got["note"].(string)
	if !strings.Contains(note, "timed out") {
		t.Errorf("note = %q, want it to say the command was cut off", note)
	}
	// The command never exited; a zero exit code would read as success.
	if code, present := got["exit_code"]; !present || code != nil {
		t.Errorf("exit_code = %v, want null for a command that timed out", code)
	}
}

func TestShellJoin(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"ls", "-la"}, "ls -la"},
		{[]string{"sh", "-c", "echo hi"}, "sh -c 'echo hi'"},
		{[]string{"echo", "it's"}, `echo 'it'\''s'`},
		{[]string{"echo", "$HOME; rm -rf /"}, "echo '$HOME; rm -rf /'"},
		{[]string{"echo", ""}, "echo ''"},
		{[]string{"echo", "=ls"}, "echo '=ls'"},
		{[]string{"env", "A=b"}, "env 'A=b'"},
	}

	for _, tt := range tests {
		if got := shellJoin(tt.args); got != tt.want {
			t.Errorf("shellJoin(%q) = %s, want %s", tt.args, got, tt.want)
		}
	}
}

func TestServiceTool_ExecService_TruncationIsReported(t *testing.T) {
	fake := &fakeExecutor{out: k8s.ExecOutput{Stdout: "lots of output", Truncated: true}}
	tool := execToolWith(fake)

	result, err := tool.Execute(context.Background(),
		[]byte(`{"environment_id":"env-123","service_name":"web","command":["cat","/var/log/app.log"]}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("expected JSON, got %q: %v", result, err)
	}

	if got["truncated"] != true {
		t.Error("expected truncated to be reported so the model knows output is incomplete")
	}
	if got["truncated_at_bytes"].(float64) != execMaxOutputBytes {
		t.Errorf("truncated_at_bytes = %v, want %d", got["truncated_at_bytes"], execMaxOutputBytes)
	}
}

// The description the model reads has to match what the tool will actually do.
func TestServiceTool_ExecService_DescriptionFollowsTheGate(t *testing.T) {
	mockClient := client.New(&servicesMockRequester{}, func() string { return "test-org" })

	disabled := NewServiceTool(mockClient, "exec_service").Definition().Description
	enabled := NewServiceToolWithExec(mockClient, "exec_service", true).Definition().Description

	if disabled == enabled {
		t.Fatal("expected the enabled tool to describe itself differently")
	}

	// A disabled tool has to say so, or a model calls it and spends a turn
	// reading setup instructions it could have read here.
	if !strings.Contains(disabled, "DISABLED") {
		t.Errorf("disabled description should say so up front, got: %s", disabled)
	}
	if !strings.Contains(disabled, "mcp.allow_exec") || !strings.Contains(disabled, "SHIPYARD_MCP_ALLOW_EXEC") {
		t.Errorf("disabled description should name both ways to enable it, got: %s", disabled)
	}
	if !strings.Contains(enabled, "non-interactive") {
		t.Errorf("enabled description should say it is non-interactive, got: %s", enabled)
	}
	if !strings.Contains(enabled, "exit code") {
		t.Errorf("enabled description should mention the exit code, got: %s", enabled)
	}
}

// The server only runs its substring-matching mapper over plain errors, and
// that mapper turns any message containing "not found" into "service not found".
// A kubeconfig failure must not reach the agent disguised as a missing service:
// it would go call get_services for a service that is right there.
func TestServiceTool_ExecService_PodFailureKeepsItsReason(t *testing.T) {
	mockClient := client.New(&servicesMockRequester{}, func() string { return "test-org" })
	tool := NewServiceToolWithExec(mockClient, "exec_service", true)
	tool.newExecutor = func(client.Client, string, *types.Service) (podExecutor, error) {
		return nil, goerrors.New("failed to retrieve kubeconfig: application kubeconfig not found!")
	}

	_, err := tool.Execute(context.Background(),
		[]byte(`{"environment_id":"env-123","service_name":"web","command":["echo","hi"]}`))
	if err == nil {
		t.Fatal("expected an error when the pod cannot be reached")
	}

	// A structured error is what the server passes through untouched.
	var mcpErr *mcperrors.MCPError
	if !goerrors.As(err, &mcpErr) {
		t.Fatalf("expected an *errors.MCPError so the server does not rewrite it, got %T", err)
	}

	if !strings.Contains(err.Error(), "kubeconfig") {
		t.Errorf("expected the real reason to survive, got: %v", err)
	}
	if strings.Contains(err.Error(), "'requested' not found") {
		t.Errorf("the reason was rewritten into a missing-service error: %v", err)
	}
}
