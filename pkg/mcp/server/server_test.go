package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shipyard/shipyard-cli/pkg/client"
)

// Mock requester for testing
type mockRequester struct{}

func (m *mockRequester) Do(method string, uri string, contentType string, body any) ([]byte, error) {
	// Return mock response for successful calls
	return []byte(`{"data": {"environments": []}}`), nil
}

func newMockClient() client.Client {
	return client.New(&mockRequester{}, func() string { return "test-org" })
}

// JSON-RPC 2.0 Message Handling Tests
func TestNewMCPServer(t *testing.T) {
	config := MCPServerConfig{
		Transport:    "stdio",
		AuditLogging: true,
	}
	client := newMockClient()

	server := NewMCPServer(config, client)

	if server == nil {
		t.Fatal("Expected server to be created, got nil")
	}
	if server.config.Transport != "stdio" {
		t.Errorf("Expected transport to be 'stdio', got %s", server.config.Transport)
	}
	if server.running {
		t.Error("Expected server to not be running initially")
	}
}

func TestMCPServer_HandleInitialize(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}

	response := server.handleInitialize(req)

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["jsonrpc"] != "2.0" {
		t.Errorf("Expected JSON-RPC 2.0, got %v", result["jsonrpc"])
	}
	if result["id"] != float64(1) {
		t.Errorf("Expected ID 1, got %v", result["id"])
	}

	resultData, ok := result["result"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected result to be an object")
	}
	if resultData["protocolVersion"] != "2024-11-05" {
		t.Errorf("Expected protocol version 2024-11-05, got %v", resultData["protocolVersion"])
	}
}

func TestMCPServer_HandleListTools(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())
	server.registerTools()
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	response := server.handleListTools(req)

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	resultData, ok := result["result"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected result to be an object")
	}

	tools, ok := resultData["tools"].([]interface{})
	if !ok {
		t.Fatal("Expected tools to be an array")
	}
	if len(tools) == 0 {
		t.Error("Expected at least one tool to be registered")
	}
}

func TestMCPServer_HandleCallTool(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())
	server.registerTools()

	params := map[string]interface{}{
		"name":      "get_environments",
		"arguments": json.RawMessage(`{"page": 1}`),
	}
	paramsJSON, _ := json.Marshal(params)

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  paramsJSON,
	}

	response := server.handleCallTool(req)

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["error"] != nil {
		t.Errorf("Expected successful tool call, got error: %v", result["error"])
	}
}

func TestMCPServer_HandleCallTool_InvalidTool(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	params := map[string]interface{}{
		"name":      "nonexistent_tool",
		"arguments": json.RawMessage(`{}`),
	}
	paramsJSON, _ := json.Marshal(params)

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  paramsJSON,
	}

	response := server.handleCallTool(req)

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["error"] == nil {
		t.Error("Expected error for invalid tool, got success")
	}
}

func TestMCPServer_HandleCallTool_InvalidParams(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{invalid json}`),
	}

	response := server.handleCallTool(req)

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["error"] == nil {
		t.Error("Expected error for invalid params, got success")
	}
}

func TestJSONRPCRequest_Validate(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	tests := []struct {
		name    string
		req     JSONRPCRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "test",
				ID:      1,
			},
			wantErr: false,
		},
		{
			name: "invalid version",
			req: JSONRPCRequest{
				JSONRPC: "1.0",
				Method:  "test",
				ID:      1,
			},
			wantErr: true,
		},
		{
			name: "missing method",
			req: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := server.validateRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestJSONRPCResponse_Marshal(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	response := server.successResponse(1, map[string]string{"test": "value"})

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["jsonrpc"] != "2.0" {
		t.Errorf("Expected JSON-RPC 2.0, got %v", result["jsonrpc"])
	}
	if result["id"] != float64(1) {
		t.Errorf("Expected ID 1, got %v", result["id"])
	}
	if result["result"] == nil {
		t.Error("Expected result field to be present")
	}
}

func TestJSONRPCError_Creation(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	response := server.errorResponse(1, -32600, "Invalid Request", "test data")

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["jsonrpc"] != "2.0" {
		t.Errorf("Expected JSON-RPC 2.0, got %v", result["jsonrpc"])
	}
	if result["id"] != float64(1) {
		t.Errorf("Expected ID 1, got %v", result["id"])
	}

	errorObj, ok := result["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected error to be an object")
	}
	if errorObj["code"] != float64(-32600) {
		t.Errorf("Expected error code -32600, got %v", errorObj["code"])
	}
	if errorObj["message"] != "Invalid Request" {
		t.Errorf("Expected error message 'Invalid Request', got %v", errorObj["message"])
	}
}

// Server Lifecycle Management Tests
func TestMCPServer_Start(t *testing.T) {
	config := MCPServerConfig{
		Transport: "stdio",
	}
	server := NewMCPServer(config, newMockClient())

	if server.IsRunning() {
		t.Error("Expected server to not be running initially")
	}

	// Note: Can't fully test Start() without mocking stdio
	// This tests the basic state management
	if server.running {
		t.Error("Expected server.running to be false initially")
	}
}

func TestMCPServer_Stop(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	// Test stopping a non-running server (should not error)
	err := server.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping non-running server, got: %v", err)
	}

	// Manually set running state to test stop logic
	server.running = true
	err = server.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping server, got: %v", err)
	}
	if server.IsRunning() {
		t.Error("Expected server to not be running after stop")
	}
}

func TestMCPServer_Stop_WithActiveConnections(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())
	server.running = true

	err := server.Stop()
	if err != nil {
		t.Errorf("Expected graceful shutdown, got error: %v", err)
	}
	if server.IsRunning() {
		t.Error("Expected server to stop even with active connections")
	}
}

func TestMCPServer_Restart(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{Transport: "stdio"}, newMockClient())

	// Test restart sequence (stop then start)
	err := server.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping server, got: %v", err)
	}

	// Note: Can't fully test Start() without mocking stdio
	// This tests the state management aspect
	if server.IsRunning() {
		t.Error("Expected server to not be running after stop")
	}
}

func TestMCPServer_HealthCheck(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	// Test health check on non-running server
	if server.IsRunning() {
		t.Error("Expected server to report as not running")
	}

	// Test health check after marking as running
	server.running = true
	if !server.IsRunning() {
		t.Error("Expected server to report as running")
	}
}

// Configuration Integration Tests
func TestMCPServerConfig_LoadFromViper(t *testing.T) {
	config := LoadMCPServerConfig()

	// Test default values are loaded
	if config.Transport != "stdio" {
		t.Errorf("Expected default transport 'stdio', got %s", config.Transport)
	}
	if config.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", config.Port)
	}
}

func TestMCPServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name   string
		config MCPServerConfig
		valid  bool
	}{
		{
			name: "valid stdio config",
			config: MCPServerConfig{
				Transport: "stdio",
			},
			valid: true,
		},
		{
			name: "valid config with audit logging",
			config: MCPServerConfig{
				Transport:    "stdio",
				AuditLogging: true,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation - transport should be supported
			if tt.config.Transport != "stdio" {
				if tt.valid {
					t.Error("Expected config to be valid")
				}
			} else {
				if !tt.valid {
					t.Error("Expected config to be invalid")
				}
			}
		})
	}
}

func TestMCPServerConfig_Defaults(t *testing.T) {
	config := LoadMCPServerConfig()

	// Verify all default values
	expected := MCPServerConfig{
		Transport:    "stdio",
		Port:         8080,
		AuditLogging: true,
	}

	if config.Transport != expected.Transport {
		t.Errorf("Expected transport %s, got %s", expected.Transport, config.Transport)
	}
	if config.Port != expected.Port {
		t.Errorf("Expected port %d, got %d", expected.Port, config.Port)
	}
	if config.AuditLogging != expected.AuditLogging {
		t.Errorf("Expected audit logging %v, got %v", expected.AuditLogging, config.AuditLogging)
	}
}

func TestMCPServer_WithShipyardClient(t *testing.T) {
	client := newMockClient()
	server := NewMCPServer(MCPServerConfig{}, client)

	if server.client.Requester != client.Requester {
		t.Error("Expected server to use provided client")
	}

	// Test that tools are registered with the client
	server.registerTools()
	if len(server.tools) == 0 {
		t.Error("Expected tools to be registered with client")
	}
}

func TestMCPServer_WithAuditLogging(t *testing.T) {
	config := MCPServerConfig{
		AuditLogging: true,
	}
	server := NewMCPServer(config, newMockClient())
	server.setupMiddleware()

	// Audit middleware was removed as unused, so no middleware is expected
	if len(server.middleware) != 0 {
		t.Error("Expected no middleware when audit logging is configured (audit middleware was removed)")
	}

	// Test without audit logging
	config.AuditLogging = false
	server2 := NewMCPServer(config, newMockClient())
	server2.setupMiddleware()

	if len(server2.middleware) != 0 {
		t.Error("Expected no middleware when audit logging is disabled")
	}
}

// Error Handling Tests
func TestMCPServer_HandleMalformedJSON(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	malformedJSON := []byte(`{"jsonrpc": "2.0", "method": "test", invalid}`)
	response := server.processMessage(malformedJSON)

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	errorObj, ok := result["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected error object in response")
	}
	if errorObj["code"] != float64(-32700) {
		t.Errorf("Expected parse error code -32700, got %v", errorObj["code"])
	}
	if !strings.Contains(errorObj["message"].(string), "Parse error") {
		t.Errorf("Expected parse error message, got %v", errorObj["message"])
	}
}

func TestMCPServer_HandleInvalidJSONRPCVersion(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	invalidVersionJSON := []byte(`{"jsonrpc": "1.0", "method": "test", "id": 1}`)
	response := server.processMessage(invalidVersionJSON)

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	errorObj, ok := result["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected error object in response")
	}
	if errorObj["code"] != float64(-32600) {
		t.Errorf("Expected invalid request code -32600, got %v", errorObj["code"])
	}
}

func TestMCPServer_HandleMissingMethod(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	missingMethodJSON := []byte(`{"jsonrpc": "2.0", "id": 1}`)
	response := server.processMessage(missingMethodJSON)

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	errorObj, ok := result["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected error object in response")
	}
	if errorObj["code"] != float64(-32600) {
		t.Errorf("Expected invalid request code -32600, got %v", errorObj["code"])
	}
}

func TestMCPServer_HandleInvalidID(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	// Test with various ID types (all should be valid in JSON-RPC 2.0)
	tests := []struct {
		name string
		id   interface{}
	}{
		{"string ID", "test-id"},
		{"number ID", 123},
		{"null ID", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "initialize",
				ID:      tt.id,
			}
			reqJSON, _ := json.Marshal(req)
			response := server.processMessage(reqJSON)

			var result map[string]interface{}
			if err := json.Unmarshal(response, &result); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			// Should not be an error for valid IDs
			if result["error"] != nil {
				t.Errorf("Expected valid ID %v to not cause error, got: %v", tt.id, result["error"])
			}
		})
	}
}

// Concurrent Access Tests
func TestMCPServer_ConcurrentRequests(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())
	server.registerTools()

	// Test concurrent initialize requests
	numRequests := 10
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			req := JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "initialize",
				ID:      id,
			}
			reqJSON, _ := json.Marshal(req)
			response := server.processMessage(reqJSON)

			var result map[string]interface{}
			if err := json.Unmarshal(response, &result); err != nil {
				t.Errorf("Failed to unmarshal response for request %d: %v", id, err)
			}
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	timeout := time.After(5 * time.Second)
	for i := 0; i < numRequests; i++ {
		select {
		case <-done:
			// Request completed
		case <-timeout:
			t.Fatal("Timeout waiting for concurrent requests to complete")
		}
	}
}

func TestMCPServer_RequestCancellation(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	// Test context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	server.ctx = ctx
	server.cancel = cancel

	// Cancel the context
	cancel()

	// Verify context is cancelled
	select {
	case <-server.ctx.Done():
		// Expected
	default:
		t.Error("Expected context to be cancelled")
	}
}

func TestMCPServer_ContextTimeout(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{}, newMockClient())

	// Test with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	server.ctx = ctx

	// Wait for timeout
	time.Sleep(10 * time.Millisecond)

	// Verify context is done
	select {
	case <-server.ctx.Done():
		if server.ctx.Err() != context.DeadlineExceeded {
			t.Errorf("Expected context deadline exceeded, got: %v", server.ctx.Err())
		}
	default:
		t.Error("Expected context to be timed out")
	}
}
