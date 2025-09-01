package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/schemas"
	"github.com/shipyard/shipyard-cli/pkg/mcp/validation"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/services/environment"
)

// toolDefinitions maps tool names to their definitions
var toolDefinitions = map[string]ToolDefinition{
	"get_environments": {
		Name:        "get_environments",
		Description: "List Shipyard environments with optional filtering",
		InputSchema: schemas.ListEnvironmentsSchema(),
	},
	"get_environment": {
		Name:        "get_environment",
		Description: "Get details for a specific environment by ID. The bypass_token in the response can be used as 'shipyard_token' URL parameter to access protected environments without login, example: https://my-environment.myorg.shipyard.host?shipyard_token=[bypass-token]",
		InputSchema: schemas.EnvironmentIDSchema(),
	},
	"restart_environment": {
		Name:        "restart_environment",
		Description: "Restart a stopped environment",
		InputSchema: schemas.EnvironmentIDSchema(),
	},
	"stop_environment": {
		Name:        "stop_environment",
		Description: "Stop a running environment",
		InputSchema: schemas.EnvironmentIDSchema(),
	},
	"cancel_environment": {
		Name:        "cancel_environment",
		Description: "Cancel an environment's latest build",
		InputSchema: schemas.EnvironmentIDSchema(),
	},
	"rebuild_environment": {
		Name:        "rebuild_environment",
		Description: "Rebuild an environment with the latest commit",
		InputSchema: schemas.EnvironmentIDSchema(),
	},
	"revive_environment": {
		Name:        "revive_environment",
		Description: "Revive a deleted environment",
		InputSchema: schemas.EnvironmentIDSchema(),
	},
}

// EnvironmentTool handles environment-related MCP operations
type EnvironmentTool struct {
	client client.Client
	name   string
}

// NewEnvironmentTool creates a new environment tool
func NewEnvironmentTool(client client.Client, name string) *EnvironmentTool {
	return &EnvironmentTool{
		client: client,
		name:   name,
	}
}

// Definition returns the tool definition for MCP
func (t *EnvironmentTool) Definition() ToolDefinition {
	if def, exists := toolDefinitions[t.name]; exists {
		return def
	}

	// Fallback for unknown tools
	return ToolDefinition{
		Name:        t.name,
		Description: "Unknown environment operation",
		InputSchema: map[string]interface{}{"type": "object"},
	}
}

// Execute runs the tool with given parameters
func (t *EnvironmentTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	log.Printf("MCP tool execution started: %s with params: %s", t.name, string(params))
	fmt.Printf("DEBUG: MCP tool execution started: %s with params: %s\n", t.name, string(params))

	switch t.name {
	case "get_environments":
		return t.executeGetEnvironments(params)
	case "get_environment":
		return t.executeGetEnvironment(params)
	case "restart_environment":
		return t.executeRestartEnvironment(params)
	case "stop_environment":
		return t.executeStopEnvironment(params)
	case "cancel_environment":
		return t.executeCancelEnvironment(params)
	case "rebuild_environment":
		return t.executeRebuildEnvironment(params)
	case "revive_environment":
		return t.executeReviveEnvironment(params)
	default:
		return "", fmt.Errorf("unknown operation: %s", t.name)
	}
}

func (t *EnvironmentTool) executeGetEnvironments(params json.RawMessage) (string, error) {
	// Parse parameters
	var toolParams struct {
		Branch   string `json:"branch,omitempty"`
		RepoName string `json:"repo_name,omitempty"`
		Deleted  bool   `json:"deleted,omitempty"`
		Page     int    `json:"page,omitempty"`
		PageSize int    `json:"page_size,omitempty"`
	}

	if len(params) > 0 {
		if err := json.Unmarshal(params, &toolParams); err != nil {
			return "", errors.ValidationError("get_environments", "parameters", err.Error())
		}
	}

	// Validate parameters
	if err := validation.ValidateBranchName(toolParams.Branch); err != nil {
		return "", errors.ValidationError("get_environments", "branch", err.Error())
	}

	if err := validation.ValidateRepoName(toolParams.RepoName); err != nil {
		return "", errors.ValidationError("get_environments", "repo_name", err.Error())
	}

	if err := validation.ValidatePagination(toolParams.Page, toolParams.PageSize); err != nil {
		return "", errors.ValidationError("get_environments", "pagination", err.Error())
	}

	// Set defaults
	if toolParams.Page == 0 {
		toolParams.Page = 1
	}
	if toolParams.PageSize == 0 {
		toolParams.PageSize = 20
	}

	// Build query parameters for direct API call
	apiParams := make(map[string]string)
	if toolParams.Branch != "" {
		apiParams["branch"] = toolParams.Branch
	}
	if toolParams.RepoName != "" {
		apiParams["repo_name"] = toolParams.RepoName
	}
	if toolParams.Deleted {
		apiParams["deleted"] = "true"
	}
	if toolParams.Page != 0 {
		apiParams["page"] = fmt.Sprintf("%d", toolParams.Page)
	}
	if toolParams.PageSize != 0 {
		apiParams["page_size"] = fmt.Sprintf("%d", toolParams.PageSize)
	}
	if t.client.OrgLookupFn != nil {
		if org := t.client.OrgLookupFn(); org != "" {
			apiParams["org"] = org
		}
	}

	// Call API directly to get raw JSON response (same as --json flag)
	if t.client.Requester == nil {
		return "", errors.NewMCPError("get_environments", "client not properly initialized", nil).
			WithSuggestion("Please ensure the MCP server is configured correctly")
	}

	body, err := t.client.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "environment", "", "", apiParams), "application/json", nil)
	if err != nil {
		log.Printf("MCP get_environments error: %v", err)
		return "", errors.ParseHTTPError("get_environments", err, "")
	}

	// Return raw JSON response (same as --json flag output)
	return string(body), nil
}

func (t *EnvironmentTool) executeGetEnvironment(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("get_environment", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("get_environment", "environment_id", err.Error())
	}

	// Build query parameters for direct API call
	apiParams := make(map[string]string)
	if t.client.OrgLookupFn != nil {
		if org := t.client.OrgLookupFn(); org != "" {
			apiParams["org"] = org
		}
	}

	// Call API directly to get raw JSON response (same as --json flag)
	if t.client.Requester == nil {
		return "", errors.NewMCPError("get_environment", "client not properly initialized", nil).
			WithSuggestion("Please ensure the MCP server is configured correctly")
	}

	body, err := t.client.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "", apiParams), "application/json", nil)
	if err != nil {
		log.Printf("MCP get_environment error: %v", err)
		return "", errors.ParseHTTPError("get_environment", err, toolParams.EnvironmentID)
	}

	// Return raw JSON response (same as --json flag output)
	return string(body), nil
}

func (t *EnvironmentTool) executeRestartEnvironment(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("restart_environment", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("restart_environment", "environment_id", err.Error())
	}

	// Use service layer for business logic
	svc := environment.NewEnvironmentManager(t.client)
	err := svc.Restart(toolParams.EnvironmentID)
	if err != nil {
		log.Printf("MCP restart_environment info: %v", err)
		// Return the API error as informational text instead of failing
		return fmt.Sprintf("Cannot restart environment %s: %s", toolParams.EnvironmentID, err.Error()), nil
	}

	return fmt.Sprintf("Environment %s queued for restart.", toolParams.EnvironmentID), nil
}

func (t *EnvironmentTool) executeStopEnvironment(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("stop_environment", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("stop_environment", "environment_id", err.Error())
	}

	// Use service layer for business logic
	svc := environment.NewEnvironmentManager(t.client)
	err := svc.Stop(toolParams.EnvironmentID)
	if err != nil {
		log.Printf("MCP stop_environment info: %v", err)
		// Return the API error as informational text instead of failing
		return fmt.Sprintf("Cannot stop environment %s: %s", toolParams.EnvironmentID, err.Error()), nil
	}

	return fmt.Sprintf("Environment %s stopped.", toolParams.EnvironmentID), nil
}

func (t *EnvironmentTool) executeCancelEnvironment(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("cancel_environment", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("cancel_environment", "environment_id", err.Error())
	}

	// Use service layer for business logic
	svc := environment.NewEnvironmentManager(t.client)
	err := svc.Cancel(toolParams.EnvironmentID)
	if err != nil {
		log.Printf("MCP cancel_environment info: %v", err)
		// Return the API error as informational text instead of failing
		return fmt.Sprintf("Cannot cancel environment %s: %s", toolParams.EnvironmentID, err.Error()), nil
	}

	return fmt.Sprintf("Environment %s build canceled.", toolParams.EnvironmentID), nil
}

func (t *EnvironmentTool) executeRebuildEnvironment(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("rebuild_environment", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("rebuild_environment", "environment_id", err.Error())
	}

	// Use service layer for business logic
	svc := environment.NewEnvironmentManager(t.client)
	err := svc.Rebuild(toolParams.EnvironmentID)
	if err != nil {
		log.Printf("MCP rebuild_environment info: %v", err)
		// Return the API error as informational text instead of failing
		return fmt.Sprintf("Cannot rebuild environment %s: %s", toolParams.EnvironmentID, err.Error()), nil
	}

	return fmt.Sprintf("Environment %s queued for rebuild.", toolParams.EnvironmentID), nil
}

func (t *EnvironmentTool) executeReviveEnvironment(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("revive_environment", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("revive_environment", "environment_id", err.Error())
	}

	// Use service layer for business logic
	svc := environment.NewEnvironmentManager(t.client)
	err := svc.Revive(toolParams.EnvironmentID)
	if err != nil {
		log.Printf("MCP revive_environment info: %v", err)
		// Return the API error as informational text instead of failing
		return fmt.Sprintf("Cannot revive environment %s: %s", toolParams.EnvironmentID, err.Error()), nil
	}

	return fmt.Sprintf("Environment %s revived successfully.", toolParams.EnvironmentID), nil
}
