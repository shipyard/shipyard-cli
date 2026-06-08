package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestEnvironmentTool_ValidationIntegration(t *testing.T) {
	client := newMockClient()

	tests := []struct {
		name      string
		toolName  string
		params    map[string]interface{}
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid environment ID",
			toolName:  "get_environment",
			params:    map[string]interface{}{"environment_id": "env-123"},
			expectErr: false,
		},
		{
			name:      "invalid environment ID - too short",
			toolName:  "get_environment",
			params:    map[string]interface{}{"environment_id": "ab"},
			expectErr: true,
			errMsg:    "environment_id too short",
		},
		{
			name:      "invalid environment ID - invalid characters",
			toolName:  "get_environment",
			params:    map[string]interface{}{"environment_id": "env/123"},
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "valid pagination",
			toolName:  "get_environments",
			params:    map[string]interface{}{"page": 1, "page_size": 20},
			expectErr: false,
		},
		{
			name:      "invalid pagination - negative page",
			toolName:  "get_environments",
			params:    map[string]interface{}{"page": -1, "page_size": 20},
			expectErr: true,
			errMsg:    "page must be non-negative",
		},
		{
			name:      "invalid pagination - page size too large",
			toolName:  "get_environments",
			params:    map[string]interface{}{"page": 1, "page_size": 2000},
			expectErr: true,
			errMsg:    "page_size 2000 too large",
		},
		{
			name:      "invalid branch name",
			toolName:  "get_environments",
			params:    map[string]interface{}{"branch": "/invalid"},
			expectErr: true,
			errMsg:    "cannot start or end with slash",
		},
		{
			name:      "invalid repo name",
			toolName:  "get_environments",
			params:    map[string]interface{}{"repo_name": "org/repo/invalid"},
			expectErr: true,
			errMsg:    "invalid format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewEnvironmentTool(client, tt.toolName)
			paramsJSON, _ := json.Marshal(tt.params)

			ctx := context.Background()
			_, err := tool.Execute(ctx, paramsJSON)

			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got %v", err)
				}
			}
		})
	}
}

func TestLogsTool_ValidationIntegration(t *testing.T) {
	client := newMockClient()
	tool := NewLogsTool(client, "get_logs")

	tests := []struct {
		name      string
		params    map[string]interface{}
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid parameters",
			params:    map[string]interface{}{"environment_id": "env-123", "service_name": "web", "tail": 100},
			expectErr: true,
			errMsg:    "network or connectivity issue", // K8s connection will fail in test environment
		},
		{
			name:      "invalid environment ID",
			params:    map[string]interface{}{"environment_id": "ab", "service_name": "web"},
			expectErr: true,
			errMsg:    "environment_id too short",
		},
		{
			name:      "invalid service name",
			params:    map[string]interface{}{"environment_id": "env-123", "service_name": "web server"},
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "invalid tail - negative",
			params:    map[string]interface{}{"environment_id": "env-123", "service_name": "web", "tail": -1},
			expectErr: true,
			errMsg:    "tail must be non-negative",
		},
		{
			name:      "invalid tail - too large",
			params:    map[string]interface{}{"environment_id": "env-123", "service_name": "web", "tail": 20000},
			expectErr: true,
			errMsg:    "tail 20000 too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paramsJSON, _ := json.Marshal(tt.params)

			ctx := context.Background()
			_, err := tool.Execute(ctx, paramsJSON)

			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got %v", err)
				}
			}
		})
	}
}
