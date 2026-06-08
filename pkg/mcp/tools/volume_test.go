package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/client"
)

// Mock requester for testing volumes
type volumesMockRequester struct{}

func (m *volumesMockRequester) Do(method, uri string, contentType string, body interface{}) ([]byte, error) {
	// Return mock JSON response for volumes
	if strings.Contains(uri, "volumes") && method == "GET" {
		return []byte(`{
			"data": [
				{
					"id": "vol-123",
					"attributes": {
						"volume_name": "data",
						"service_name": "web",
						"volume_path": "/app/data",
						"compose_path": "./data"
					}
				},
				{
					"id": "vol-456",
					"attributes": {
						"volume_name": "logs",
						"service_name": "api",
						"volume_path": "/var/log",
						"compose_path": "./logs"
					}
				}
			]
		}`), nil
	}

	// Return mock JSON response for snapshots
	if strings.Contains(uri, "volume-snapshots") && method == "GET" {
		return []byte(`{
			"data": [
				{
					"id": "snap-123",
					"type": "snapshot",
					"attributes": {
						"sequence_number": 5,
						"from_snapshot_number": 0,
						"status": "completed",
						"created_at": "2023-12-01T10:00:00Z"
					}
				},
				{
					"id": "snap-456",
					"type": "snapshot",
					"attributes": {
						"sequence_number": 6,
						"from_snapshot_number": 5,
						"status": "completed",
						"created_at": "2023-12-01T11:00:00Z"
					}
				}
			],
			"links": {
				"next": ""
			}
		}`), nil
	}

	// Mock successful POST responses for other operations
	if method == "POST" {
		return []byte(`{"success": true}`), nil
	}

	return nil, nil
}

func TestVolumeTool_Definition(t *testing.T) {
	tests := []struct {
		name         string
		toolName     string
		expectedName string
		expectedDesc string
	}{
		{
			name:         "get_volumes tool definition",
			toolName:     "get_volumes",
			expectedName: "get_volumes",
			expectedDesc: "List volumes in an environment",
		},
		{
			name:         "get_snapshots tool definition",
			toolName:     "get_snapshots",
			expectedName: "get_snapshots",
			expectedDesc: "List volume snapshots in an environment",
		},
		{
			name:         "reset_volume tool definition",
			toolName:     "reset_volume",
			expectedName: "reset_volume",
			expectedDesc: "Reset volume to initial state",
		},
		{
			name:         "create_snapshot tool definition",
			toolName:     "create_snapshot",
			expectedName: "create_snapshot",
			expectedDesc: "Create volume snapshot",
		},
		{
			name:         "load_snapshot tool definition",
			toolName:     "load_snapshot",
			expectedName: "load_snapshot",
			expectedDesc: "Load volume snapshot",
		},
	}

	// Create mock client for testing
	mockClient := client.New(&volumesMockRequester{}, func() string { return "test-org" })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewVolumeTool(mockClient, tt.toolName)
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

func TestVolumeTool_Execute_GetVolumes(t *testing.T) {
	mockClient := client.New(&volumesMockRequester{}, func() string { return "test-org" })
	tool := NewVolumeTool(mockClient, "get_volumes")

	result, err := tool.Execute(context.Background(), []byte(`{"environment_id":"env-123"}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify it returns JSON format
	var jsonResult map[string]interface{}
	if err := json.Unmarshal([]byte(result), &jsonResult); err != nil {
		t.Errorf("Expected valid JSON result, got: %s", result)
	}

	// Verify the JSON contains expected volume data
	expectedSubstrings := []string{
		"data",
		"web",
		"logs",
		"api",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(result, substr) {
			t.Errorf("Expected result to contain %q, got: %s", substr, result)
		}
	}
}

func TestVolumeTool_Execute_GetSnapshots(t *testing.T) {
	mockClient := client.New(&volumesMockRequester{}, func() string { return "test-org" })
	tool := NewVolumeTool(mockClient, "get_snapshots")

	result, err := tool.Execute(context.Background(), []byte(`{"environment_id":"env-123"}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify it returns JSON format
	var jsonResult map[string]interface{}
	if err := json.Unmarshal([]byte(result), &jsonResult); err != nil {
		t.Errorf("Expected valid JSON result, got: %s", result)
	}

	// Verify the JSON contains expected snapshot data
	expectedSubstrings := []string{
		"snap-123",
		"snap-456",
		"sequence_number",
		"completed",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(result, substr) {
			t.Errorf("Expected result to contain %q, got: %s", substr, result)
		}
	}
}

func TestVolumeTool_Execute_ResetVolume(t *testing.T) {
	mockClient := client.New(&volumesMockRequester{}, func() string { return "test-org" })
	tool := NewVolumeTool(mockClient, "reset_volume")

	result, err := tool.Execute(context.Background(), []byte(`{"environment_id":"env-123","volume_name":"data"}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expectedSubstrings := []string{
		"Volume 'data' in environment env-123 has been reset",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(result, substr) {
			t.Errorf("Expected result to contain %q, got: %s", substr, result)
		}
	}
}

func TestVolumeTool_Execute_CreateSnapshot(t *testing.T) {
	mockClient := client.New(&volumesMockRequester{}, func() string { return "test-org" })
	tool := NewVolumeTool(mockClient, "create_snapshot")

	result, err := tool.Execute(context.Background(), []byte(`{"environment_id":"env-123","note":"Test snapshot"}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expectedSubstrings := []string{
		"Snapshot created for environment env-123",
		"Note: Test snapshot",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(result, substr) {
			t.Errorf("Expected result to contain %q, got: %s", substr, result)
		}
	}
}

func TestVolumeTool_Execute_LoadSnapshot(t *testing.T) {
	mockClient := client.New(&volumesMockRequester{}, func() string { return "test-org" })
	tool := NewVolumeTool(mockClient, "load_snapshot")

	result, err := tool.Execute(context.Background(), []byte(`{"environment_id":"env-123","sequence_number":5}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expectedSubstrings := []string{
		"Snapshot 5 loaded into environment env-123",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(result, substr) {
			t.Errorf("Expected result to contain %q, got: %s", substr, result)
		}
	}
}

func TestVolumeTool_Execute_InvalidParams(t *testing.T) {
	mockClient := client.New(&volumesMockRequester{}, func() string { return "test-org" })
	tool := NewVolumeTool(mockClient, "get_volumes")

	_, err := tool.Execute(context.Background(), []byte(`{"invalid": "params"}`))
	if err == nil {
		t.Fatal("Expected error for invalid environment_id")
	}

	if !strings.Contains(err.Error(), "invalid environment_id") {
		t.Errorf("Expected error to mention invalid environment_id, got: %v", err)
	}
}
