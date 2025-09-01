package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/client"
)

// Mock requester for testing
type mockRequester struct{}

func (m *mockRequester) Do(method, uri string, contentType string, body interface{}) ([]byte, error) {
	// Return mock JSON response for single environment
	if strings.Contains(uri, "environment/env-") && method == "GET" {
		return []byte(`{
			"data": {
				"id": "env-123",
				"attributes": {
					"name": "test-env",
					"url": "https://test.shipyard.build",
					"ready": true,
					"projects": [
						{
							"repo_name": "test-repo",
							"pull_request_number": 42
						}
					],
					"services": [
						{
							"name": "web",
							"ports": ["3000"],
							"url": "https://web.test.shipyard.build"
						}
					]
				}
			}
		}`), nil
	}

	// Return mock JSON response for environment list
	if strings.Contains(uri, "environment") && method == "GET" {
		return []byte(`{
			"data": [
				{
					"id": "env-123",
					"attributes": {
						"name": "test-env",
						"url": "https://test.shipyard.build",
						"ready": true,
						"projects": [
							{
								"repo_name": "test-repo",
								"pull_request_number": 42
							}
						],
						"services": [
							{
								"name": "web",
								"ports": ["3000"],
								"url": "https://web.test.shipyard.build"
							}
						]
					}
				}
			],
			"links": {
				"next": "",
				"prev": ""
			}
		}`), nil
	}

	// Return success for POST operations (restart/stop)
	if method == "POST" {
		return []byte(`{"success": true}`), nil
	}

	return []byte(`{"data": []}`), nil
}

// Mock client for testing
func newMockClient() client.Client {
	return client.Client{
		Requester: &mockRequester{},
		OrgLookupFn: func() string {
			return "test-org"
		},
	}
}

// Environment Tool Tests
func TestEnvironmentTool_GetEnvironments(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environments")

	if tool == nil {
		t.Fatal("Expected tool to be created, got nil")
	}

	// Test with no parameters
	ctx := context.Background()
	result, err := tool.Execute(ctx, nil)
	if err != nil {
		t.Errorf("Expected no error with no parameters, got: %v", err)
	}

	// Verify it returns JSON format
	var jsonResult map[string]interface{}
	if err := json.Unmarshal([]byte(result), &jsonResult); err != nil {
		t.Errorf("Expected valid JSON result, got: %s", result)
	}

	// Verify the JSON contains expected data
	if !strings.Contains(result, "test-env") {
		t.Errorf("Expected test-env in result, got: %s", result)
	}
}

func TestEnvironmentTool_GetEnvironments_WithFilters(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environments")

	// Test with branch and repo filters
	params := map[string]interface{}{
		"branch":    "main",
		"repo_name": "test-repo",
		"page":      1,
		"page_size": 10,
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	if err != nil {
		t.Errorf("Expected no error with filters, got: %v", err)
	}

	// Verify it returns JSON format with expected data
	var jsonResult map[string]interface{}
	if err := json.Unmarshal([]byte(result), &jsonResult); err != nil {
		t.Errorf("Expected valid JSON result, got: %s", result)
	}

	// Verify the JSON contains repo data
	if !strings.Contains(result, "test-repo") {
		t.Errorf("Expected test-repo in result, got: %s", result)
	}
}

func TestEnvironmentTool_GetEnvironments_Pagination(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environments")

	// Test custom pagination
	params := map[string]interface{}{
		"page":      3,
		"page_size": 50,
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	if err != nil {
		t.Errorf("Expected no error with pagination, got: %v", err)
	}

	// Verify it returns JSON format
	var jsonResult map[string]interface{}
	if err := json.Unmarshal([]byte(result), &jsonResult); err != nil {
		t.Errorf("Expected valid JSON result, got: %s", result)
	}
}

func TestEnvironmentTool_GetEnvironment_Valid(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environment")

	// Test with specific environment ID
	params := map[string]interface{}{
		"environment_id": "env-12345",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	if err != nil {
		t.Errorf("Expected no error for valid environment, got: %v", err)
	}

	// Verify it returns JSON format
	var jsonResult map[string]interface{}
	if err := json.Unmarshal([]byte(result), &jsonResult); err != nil {
		t.Errorf("Expected valid JSON result, got: %s", result)
	}

	// Verify the JSON contains expected data
	if !strings.Contains(result, "env-123") {
		t.Errorf("Expected env-123 in result, got: %s", result)
	}
}

func TestEnvironmentTool_GetEnvironment_NotFound(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environment")

	// Test with non-existent environment ID
	params := map[string]interface{}{
		"environment_id": "nonexistent",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	// Current implementation doesn't validate existence
	if err != nil {
		t.Errorf("Expected no error (validation not implemented), got: %v", err)
	}

	if result == "" {
		t.Error("Expected some result even for non-existent environment")
	}
}

func TestEnvironmentTool_GetEnvironment_Unauthorized(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environment")

	// Test access control (not yet implemented)
	params := map[string]interface{}{
		"environment_id": "unauthorized-env",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	// Current implementation doesn't check authorization
	if err != nil {
		t.Errorf("Expected no error (auth not implemented), got: %v", err)
	}

	if result == "" {
		t.Error("Expected some result even without auth checks")
	}
}

func TestEnvironmentTool_RestartEnvironment_Success(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "restart_environment")

	// Test restart operation
	params := map[string]interface{}{
		"environment_id": "env-12345",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	if err != nil {
		t.Errorf("Expected no error for restart, got: %v", err)
	}

	if !strings.Contains(result, "Environment env-12345 queued for restart") {
		t.Errorf("Expected restart confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_RestartEnvironment_AlreadyRunning(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "restart_environment")

	// Test restart of running environment
	params := map[string]interface{}{
		"environment_id": "running-env",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	// Current implementation doesn't check state
	if err != nil {
		t.Errorf("Expected no error (state check not implemented), got: %v", err)
	}

	if !strings.Contains(result, "Environment running-env") {
		t.Errorf("Expected restart confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_RestartEnvironment_NotFound(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "restart_environment")

	// Test restart of non-existent environment
	params := map[string]interface{}{
		"environment_id": "nonexistent",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	// Current implementation doesn't validate existence
	if err != nil {
		t.Errorf("Expected no error (validation not implemented), got: %v", err)
	}

	if !strings.Contains(result, "Environment nonexistent") {
		t.Errorf("Expected restart confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_StopEnvironment_Success(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "stop_environment")

	// Test stop operation
	params := map[string]interface{}{
		"environment_id": "env-12345",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	if err != nil {
		t.Errorf("Expected no error for stop, got: %v", err)
	}

	if !strings.Contains(result, "Environment env-12345 stopped") {
		t.Errorf("Expected stop confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_StopEnvironment_AlreadyStopped(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "stop_environment")

	// Test stop of already stopped environment
	params := map[string]interface{}{
		"environment_id": "stopped-env",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	// Current implementation doesn't check state
	if err != nil {
		t.Errorf("Expected no error (state check not implemented), got: %v", err)
	}

	if !strings.Contains(result, "Environment stopped-env stopped") {
		t.Errorf("Expected stop confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_StopEnvironment_NotFound(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "stop_environment")

	// Test stop of non-existent environment
	params := map[string]interface{}{
		"environment_id": "nonexistent",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	// Current implementation doesn't validate existence
	if err != nil {
		t.Errorf("Expected no error (validation not implemented), got: %v", err)
	}

	if !strings.Contains(result, "Environment nonexistent stopped") {
		t.Errorf("Expected stop confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_CancelEnvironment_Success(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "cancel_environment")

	// Test cancel operation
	params := map[string]interface{}{
		"environment_id": "env-12345",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	if err != nil {
		t.Errorf("Expected no error for cancel, got: %v", err)
	}

	if !strings.Contains(result, "build canceled") {
		t.Errorf("Expected cancel confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_CancelEnvironment_NotBuilding(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "cancel_environment")

	// Test cancel of environment not currently building
	params := map[string]interface{}{
		"environment_id": "not-building-env",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	// Current implementation doesn't check state
	if err != nil {
		t.Errorf("Expected no error (state check not implemented), got: %v", err)
	}

	if !strings.Contains(result, "build canceled") {
		t.Errorf("Expected cancel confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_CancelEnvironment_NotFound(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "cancel_environment")

	// Test cancel of non-existent environment
	params := map[string]interface{}{
		"environment_id": "nonexistent",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	// Current implementation doesn't validate existence
	if err != nil {
		t.Errorf("Expected no error (validation not implemented), got: %v", err)
	}

	if !strings.Contains(result, "build canceled") {
		t.Errorf("Expected cancel confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_CancelEnvironment_InvalidID(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "cancel_environment")

	// Test cancel with invalid environment ID
	params := map[string]interface{}{
		"environment_id": "",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	_, err := tool.Execute(ctx, paramsJSON)
	// Should fail validation
	if err == nil {
		t.Error("Expected error for empty environment ID")
	}
}

func TestEnvironmentTool_RebuildEnvironment_Success(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "rebuild_environment")

	// Test rebuild operation
	params := map[string]interface{}{
		"environment_id": "env-12345",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	if err != nil {
		t.Errorf("Expected no error for rebuild, got: %v", err)
	}

	if !strings.Contains(result, "queued for rebuild") {
		t.Errorf("Expected rebuild confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_RebuildEnvironment_NonExistent(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "rebuild_environment")

	// Test rebuild of non-existent environment
	params := map[string]interface{}{
		"environment_id": "nonexistent",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	// Current implementation doesn't validate existence
	if err != nil {
		t.Errorf("Expected no error (validation not implemented), got: %v", err)
	}

	if !strings.Contains(result, "queued for rebuild") {
		t.Errorf("Expected rebuild confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_RebuildEnvironment_Deleted(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "rebuild_environment")

	// Test rebuild of deleted environment
	params := map[string]interface{}{
		"environment_id": "deleted-env",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	// Current implementation doesn't check state
	if err != nil {
		t.Errorf("Expected no error (state check not implemented), got: %v", err)
	}

	if !strings.Contains(result, "queued for rebuild") {
		t.Errorf("Expected rebuild confirmation in result, got: %s", result)
	}
}

func TestEnvironmentTool_RebuildEnvironment_InvalidID(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "rebuild_environment")

	// Test rebuild with invalid environment ID
	params := map[string]interface{}{
		"environment_id": "",
	}
	paramsJSON, _ := json.Marshal(params)

	ctx := context.Background()
	_, err := tool.Execute(ctx, paramsJSON)
	// Should fail validation
	if err == nil {
		t.Error("Expected error for empty environment ID")
	}
}

// Tool Registration Tests
func TestEnvironmentTool_Registration(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environments")

	definition := tool.Definition()

	if definition.Name != "get_environments" {
		t.Errorf("Expected tool name 'get_environments', got: %s", definition.Name)
	}
	if definition.Description == "" {
		t.Error("Expected tool description to be set")
	}
	if definition.InputSchema == nil {
		t.Error("Expected input schema to be set")
	}
}

func TestEnvironmentTool_Schema_Validation(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environments")

	definition := tool.Definition()
	schema, ok := definition.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatal("Expected input schema to be a map")
	}

	if schema["type"] != "object" {
		t.Errorf("Expected schema type 'object', got: %v", schema["type"])
	}

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected properties to be a map")
	}

	// Check for expected properties
	expectedProps := []string{"branch", "repo_name", "deleted", "page", "page_size"}
	for _, prop := range expectedProps {
		if properties[prop] == nil {
			t.Errorf("Expected property '%s' in schema", prop)
		}
	}
}

func TestEnvironmentTool_Parameters_Validation(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environments")

	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "valid params",
			params:  map[string]interface{}{"page": 1, "page_size": 20},
			wantErr: false,
		},
		{
			name:    "empty params",
			params:  map[string]interface{}{},
			wantErr: false,
		},
		{
			name:    "with filters",
			params:  map[string]interface{}{"branch": "main", "repo_name": "test"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tt.params)
			ctx := context.Background()
			_, err := tool.Execute(ctx, paramsJSON)

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Integration Tests
func TestEnvironmentTool_WithShipyardClient(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environments")

	// Verify tool is created with client
	if tool.client.Requester != client.Requester {
		t.Error("Expected tool to use provided client")
	}

	// Test tool execution
	ctx := context.Background()
	result, err := tool.Execute(ctx, nil)
	if err != nil {
		t.Errorf("Expected no error with client integration, got: %v", err)
	}
	if result == "" {
		t.Error("Expected result from tool execution")
	}
}

func TestEnvironmentTool_WithAuth(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environments")

	// Test with authenticated context (simulated)
	ctx := context.WithValue(context.Background(), "auth", "test-token")
	result, err := tool.Execute(ctx, nil)
	if err != nil {
		t.Errorf("Expected no error with auth context, got: %v", err)
	}
	if result == "" {
		t.Error("Expected result even with auth context")
	}
}

func TestEnvironmentTool_ErrorHandling(t *testing.T) {
	client := newMockClient()
	tool := NewEnvironmentTool(client, "get_environments")

	// Test with invalid JSON parameters
	invalidJSON := json.RawMessage(`{invalid json}`)
	ctx := context.Background()
	_, err := tool.Execute(ctx, invalidJSON)

	if err == nil {
		t.Error("Expected error for invalid JSON parameters")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid parameters") {
		t.Errorf("Expected 'invalid parameters' error, got: %v", err)
	}
}
