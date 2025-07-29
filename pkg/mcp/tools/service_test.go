package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/client"
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
			name:         "exec_service tool definition",
			toolName:     "exec_service",
			expectedName: "exec_service",
			expectedDesc: "Execute commands in service containers",
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

	expectedSubstrings := []string{
		"Cannot execute commands interactively via MCP",
		"shipyard exec --env env-123 --service web",
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
