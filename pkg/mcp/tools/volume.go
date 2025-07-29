package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/schemas"
	"github.com/shipyard/shipyard-cli/pkg/mcp/validation"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

// volumeToolDefinitions maps volume tool names to their definitions
var volumeToolDefinitions = map[string]ToolDefinition{
	"get_volumes": {
		Name:        "get_volumes",
		Description: "List volumes in an environment",
		InputSchema: schemas.EnvironmentIDSchema(),
	},
	"get_snapshots": {
		Name:        "get_snapshots",
		Description: "List volume snapshots in an environment",
		InputSchema: schemas.SnapshotsListSchema(),
	},
	"reset_volume": {
		Name:        "reset_volume",
		Description: "Reset volume to initial state",
		InputSchema: schemas.VolumeResetSchema(),
	},
	"create_snapshot": {
		Name:        "create_snapshot",
		Description: "Create volume snapshot",
		InputSchema: schemas.SnapshotCreateSchema(),
	},
	"load_snapshot": {
		Name:        "load_snapshot",
		Description: "Load volume snapshot",
		InputSchema: schemas.SnapshotLoadSchema(),
	},
}

// VolumeTool handles volume-related MCP operations
type VolumeTool struct {
	client client.Client
	name   string
}

// NewVolumeTool creates a new volume tool
func NewVolumeTool(client client.Client, name string) *VolumeTool {
	return &VolumeTool{
		client: client,
		name:   name,
	}
}

// Definition returns the tool definition for MCP
func (t *VolumeTool) Definition() ToolDefinition {
	if def, exists := volumeToolDefinitions[t.name]; exists {
		return def
	}

	// Fallback for unknown tools
	return ToolDefinition{
		Name:        t.name,
		Description: "Unknown volume operation",
		InputSchema: map[string]interface{}{"type": "object"},
	}
}

// Execute runs the tool with given parameters
func (t *VolumeTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	log.Printf("MCP volume tool execution started: %s with params: %s", t.name, string(params))

	switch t.name {
	case "get_volumes":
		return t.executeGetVolumes(params)
	case "get_snapshots":
		return t.executeGetSnapshots(params)
	case "reset_volume":
		return t.executeResetVolume(params)
	case "create_snapshot":
		return t.executeCreateSnapshot(params)
	case "load_snapshot":
		return t.executeLoadSnapshot(params)
	default:
		return "", fmt.Errorf("unknown volume operation: %s", t.name)
	}
}

func (t *VolumeTool) executeGetVolumes(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("get_volumes", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("get_volumes", "environment_id", err.Error())
	}

	// Build request parameters
	requestParams := make(map[string]string)
	if t.client.OrgLookupFn != nil {
		if org := t.client.OrgLookupFn(); org != "" {
			requestParams["org"] = org
		}
	}

	// Call API directly to get raw JSON response (same as --json flag)
	body, err := t.client.Requester.Do(
		http.MethodGet,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "volumes", requestParams),
		"application/json",
		nil,
	)
	if err != nil {
		log.Printf("MCP get_volumes error: %v", err)
		return "", errors.ParseHTTPError("get_volumes", err, toolParams.EnvironmentID)
	}

	// Return raw JSON response (same as --json flag output)
	return string(body), nil
}

func (t *VolumeTool) executeGetSnapshots(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		Page          int    `json:"page,omitempty"`
		PageSize      int    `json:"page_size,omitempty"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("get_snapshots", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("get_snapshots", "environment_id", err.Error())
	}

	// Set defaults
	if toolParams.Page == 0 {
		toolParams.Page = 1
	}
	if toolParams.PageSize == 0 {
		toolParams.PageSize = 20
	}

	// Validate pagination
	if err := validation.ValidatePagination(toolParams.Page, toolParams.PageSize); err != nil {
		return "", errors.ValidationError("get_snapshots", "pagination", err.Error())
	}

	// Build request parameters
	requestParams := make(map[string]string)
	if t.client.OrgLookupFn != nil {
		if org := t.client.OrgLookupFn(); org != "" {
			requestParams["org"] = org
		}
	}
	requestParams["page"] = strconv.Itoa(toolParams.Page)
	requestParams["page_size"] = strconv.Itoa(toolParams.PageSize)

	// Call API directly to get raw JSON response (same as --json flag)
	body, err := t.client.Requester.Do(
		http.MethodGet,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "volume-snapshots", requestParams),
		"application/json",
		nil,
	)
	if err != nil {
		log.Printf("MCP get_snapshots error: %v", err)
		return "", errors.ParseHTTPError("get_snapshots", err, toolParams.EnvironmentID)
	}

	// Return raw JSON response (same as --json flag output)
	return string(body), nil
}

func (t *VolumeTool) executeResetVolume(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		VolumeName    string `json:"volume_name"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("reset_volume", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("reset_volume", "environment_id", err.Error())
	}

	if toolParams.VolumeName == "" {
		return "", errors.ValidationError("reset_volume", "volume_name", "volume_name is required. Example: 'data', 'uploads', 'cache'")
	}

	// Build request parameters
	requestParams := make(map[string]string)
	if org := t.client.OrgLookupFn(); org != "" {
		requestParams["org"] = org
	}

	// Make API call
	subresource := fmt.Sprintf("volume/%s/volume-reset", toolParams.VolumeName)
	_, err := t.client.Requester.Do(
		http.MethodPost,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, subresource, requestParams),
		"application/json",
		nil,
	)
	if err != nil {
		log.Printf("MCP reset_volume error: %v", err)
		return "", errors.ParseHTTPError("reset_volume", err, toolParams.EnvironmentID)
	}

	return fmt.Sprintf("Volume '%s' in environment %s has been reset to its initial state.", toolParams.VolumeName, toolParams.EnvironmentID), nil
}

func (t *VolumeTool) executeCreateSnapshot(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		Note          string `json:"note,omitempty"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("create_snapshot", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("create_snapshot", "environment_id", err.Error())
	}

	// Build request parameters
	requestParams := make(map[string]string)
	if org := t.client.OrgLookupFn(); org != "" {
		requestParams["org"] = org
	}

	// Build request body
	requestBody := map[string]interface{}{
		"note": toolParams.Note,
	}

	// Make API call
	_, err := t.client.Requester.Do(
		http.MethodPost,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "snapshot-create", requestParams),
		"application/json",
		requestBody,
	)
	if err != nil {
		log.Printf("MCP create_snapshot error: %v", err)
		return "", errors.ParseHTTPError("create_snapshot", err, toolParams.EnvironmentID)
	}

	result := fmt.Sprintf("Snapshot created for environment %s.", toolParams.EnvironmentID)
	if toolParams.Note != "" {
		result += fmt.Sprintf(" Note: %s", toolParams.Note)
	}

	return result, nil
}

func (t *VolumeTool) executeLoadSnapshot(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID       string `json:"environment_id"`
		SequenceNumber      int    `json:"sequence_number"`
		SourceApplicationID string `json:"source_application_id,omitempty"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("load_snapshot", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("load_snapshot", "environment_id", err.Error())
	}

	if toolParams.SequenceNumber <= 0 {
		return "", errors.ValidationError("load_snapshot", "sequence_number", "sequence_number must be greater than 0. Use the sequence number from a snapshot listing")
	}

	// Build request parameters
	requestParams := make(map[string]string)
	if org := t.client.OrgLookupFn(); org != "" {
		requestParams["org"] = org
	}

	// Build request body
	requestBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "snapshot-load",
			"attributes": map[string]interface{}{
				"sequence_number": toolParams.SequenceNumber,
			},
		},
	}

	if toolParams.SourceApplicationID != "" {
		attrs := requestBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
		attrs["source_application_id"] = toolParams.SourceApplicationID
	}

	// Make API call
	_, err := t.client.Requester.Do(
		http.MethodPost,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "snapshot-load", requestParams),
		"application/json",
		requestBody,
	)
	if err != nil {
		log.Printf("MCP load_snapshot error: %v", err)
		return "", errors.ParseHTTPError("load_snapshot", err, toolParams.EnvironmentID)
	}

	result := fmt.Sprintf("Snapshot %d loaded into environment %s.", toolParams.SequenceNumber, toolParams.EnvironmentID)
	if toolParams.SourceApplicationID != "" {
		result += fmt.Sprintf(" Source application: %s", toolParams.SourceApplicationID)
	}

	return result, nil
}
