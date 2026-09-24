package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/schemas"
	"github.com/shipyard/shipyard-cli/pkg/mcp/validation"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

var extendedToolDefinitions = map[string]ToolDefinition{
	"get_build_history": {
		Name:        "get_build_history",
		Description: "Get build history for an environment (queued commits and success flags)",
		InputSchema: schemas.BuildHistorySchema(),
	},
	"get_env_vars": {
		Name:        "get_env_vars",
		Description: "List environment variables for an environment (hidden values are masked)",
		InputSchema: schemas.GetEnvVarsSchema(),
	},
	"put_env_vars": {
		Name:        "put_env_vars",
		Description: "Create or update environment variables on an environment. Requires env_vars array with name and value. Partial failures return HTTP 207 semantics in the body.",
		InputSchema: schemas.PutEnvVarsSchema(),
	},
	"delete_env_var": {
		Name:        "delete_env_var",
		Description: "Delete an environment variable by name",
		InputSchema: schemas.DeleteEnvVarSchema(),
	},
	"restart_service": {
		Name:        "restart_service",
		Description: "Restart a single service in an environment without rebuilding the whole environment",
		InputSchema: schemas.RestartServiceSchema(),
	},
	"deploy_detached": {
		Name:        "deploy_detached",
		Description: "Deploy a detached environment by cloning an existing application build. Optional display_name, project_branch_overrides, and build_on_commit.",
		InputSchema: schemas.DeployDetachedSchema(),
	},
	"update_branches": {
		Name:        "update_branches",
		Description: "Change branches for an environment. You must include every repo in projects (repo_name + branch); partial lists fail.",
		InputSchema: schemas.UpdateBranchesSchema(),
	},
}

// ExtendedTool handles additional typed environment/API MCP operations.
type ExtendedTool struct {
	client client.Client
	name   string
}

// NewExtendedTool creates a typed MCP tool for the given name.
func NewExtendedTool(client client.Client, name string) *ExtendedTool {
	return &ExtendedTool{client: client, name: name}
}

// Definition returns the MCP tool definition.
func (t *ExtendedTool) Definition() ToolDefinition {
	if def, ok := extendedToolDefinitions[t.name]; ok {
		return def
	}
	return ToolDefinition{
		Name:        t.name,
		Description: "Unknown operation",
		InputSchema: map[string]interface{}{"type": "object"},
	}
}

// Execute runs the named tool.
func (t *ExtendedTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	if t.name == "put_env_vars" {
		// Params carry env var values, which are often secrets.
		log.Printf("MCP tool execution started: %s", t.name)
	} else {
		log.Printf("MCP tool execution started: %s with params: %s", t.name, string(params))
	}
	switch t.name {
	case "get_build_history":
		return t.executeGetBuildHistory(params)
	case "get_env_vars":
		return t.executeGetEnvVars(params)
	case "put_env_vars":
		return t.executePutEnvVars(params)
	case "delete_env_var":
		return t.executeDeleteEnvVar(params)
	case "restart_service":
		return t.executeRestartService(params)
	case "deploy_detached":
		return t.executeDeployDetached(params)
	case "update_branches":
		return t.executeUpdateBranches(params)
	default:
		return "", fmt.Errorf("unknown operation: %s", t.name)
	}
}

func (t *ExtendedTool) orgParams() map[string]string {
	params := make(map[string]string)
	if t.client.OrgLookupFn != nil {
		if org := t.client.OrgLookupFn(); org != "" {
			params["org"] = org
		}
	}
	return params
}

func (t *ExtendedTool) executeGetBuildHistory(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID     string `json:"environment_id"`
		SuccessfullyBuilt bool   `json:"successfully_built,omitempty"`
		Page              int    `json:"page,omitempty"`
		PageSize          int    `json:"page_size,omitempty"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &toolParams); err != nil {
			return "", errors.ValidationError("get_build_history", "parameters", err.Error())
		}
	}
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("get_build_history", "environment_id", err.Error())
	}
	if err := validation.ValidatePagination(toolParams.Page, toolParams.PageSize); err != nil {
		return "", errors.ValidationError("get_build_history", "pagination", err.Error())
	}
	if toolParams.Page == 0 {
		toolParams.Page = 1
	}
	if toolParams.PageSize == 0 {
		toolParams.PageSize = 20
	}

	apiParams := t.orgParams()
	apiParams["page"] = strconv.Itoa(toolParams.Page)
	apiParams["page_size"] = strconv.Itoa(toolParams.PageSize)
	if toolParams.SuccessfullyBuilt {
		apiParams["successfully_built"] = "true"
	}

	body, err := t.client.Requester.Do(
		http.MethodGet,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "build-history", apiParams),
		"application/json",
		nil,
	)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (t *ExtendedTool) executeGetEnvVars(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &toolParams); err != nil {
			return "", errors.ValidationError("get_env_vars", "parameters", err.Error())
		}
	}
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("get_env_vars", "environment_id", err.Error())
	}

	body, err := t.client.Requester.Do(
		http.MethodGet,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "env-vars", t.orgParams()),
		"application/json",
		nil,
	)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (t *ExtendedTool) executePutEnvVars(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		EnvVars       []struct {
			Name     string   `json:"name"`
			Value    string   `json:"value"`
			Hidden   *bool    `json:"hidden,omitempty"`
			Services []string `json:"services,omitempty"`
		} `json:"env_vars"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("put_env_vars", "parameters", err.Error())
	}
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("put_env_vars", "environment_id", err.Error())
	}
	if len(toolParams.EnvVars) == 0 {
		return "", errors.ValidationError("put_env_vars", "env_vars", "env_vars array is required and must not be empty")
	}

	payloadVars := make([]map[string]any, 0, len(toolParams.EnvVars))
	for i, v := range toolParams.EnvVars {
		if v.Name == "" {
			return "", errors.ValidationError("put_env_vars", "env_vars", fmt.Sprintf("env_vars[%d].name is required", i))
		}
		entry := map[string]any{"name": v.Name, "value": v.Value}
		if v.Hidden != nil {
			entry["hidden"] = *v.Hidden
		}
		if v.Services != nil {
			entry["services"] = v.Services
		}
		payloadVars = append(payloadVars, entry)
	}

	body, err := t.client.Requester.Do(
		http.MethodPut,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "env-vars", t.orgParams()),
		"application/json",
		map[string]any{"env_vars": payloadVars},
	)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (t *ExtendedTool) executeDeleteEnvVar(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		Name          string `json:"name"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("delete_env_var", "parameters", err.Error())
	}
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("delete_env_var", "environment_id", err.Error())
	}
	if toolParams.Name == "" {
		return "", errors.ValidationError("delete_env_var", "name", "name is required")
	}

	subresource := fmt.Sprintf("env-vars/%s", url.PathEscape(toolParams.Name))
	_, err := t.client.Requester.Do(
		http.MethodDelete,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, subresource, t.orgParams()),
		"application/json",
		nil,
	)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Deleted env var %q on environment %s", toolParams.Name, toolParams.EnvironmentID), nil
}

func (t *ExtendedTool) executeRestartService(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		ServiceName   string `json:"service_name"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("restart_service", "parameters", err.Error())
	}
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("restart_service", "environment_id", err.Error())
	}
	if err := validation.ValidateServiceName(toolParams.ServiceName); err != nil {
		return "", errors.ValidationError("restart_service", "service_name", err.Error())
	}

	subresource := fmt.Sprintf("service/%s/restart", toolParams.ServiceName)
	_, err := t.client.Requester.Do(
		http.MethodPost,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, subresource, t.orgParams()),
		"application/json",
		nil,
	)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Restarted service %q on environment %s", toolParams.ServiceName, toolParams.EnvironmentID), nil
}

func (t *ExtendedTool) executeDeployDetached(params json.RawMessage) (string, error) {
	var toolParams struct {
		ApplicationBuildID     string            `json:"application_build_id"`
		DisplayName            string            `json:"display_name,omitempty"`
		ProjectBranchOverrides map[string]string `json:"project_branch_overrides,omitempty"`
		BuildOnCommit          json.RawMessage   `json:"build_on_commit,omitempty"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("deploy_detached", "parameters", err.Error())
	}
	if err := validation.ValidateEnvironmentID(toolParams.ApplicationBuildID); err != nil {
		return "", errors.ValidationError("deploy_detached", "application_build_id", err.Error())
	}

	payload := make(map[string]any)
	if toolParams.DisplayName != "" {
		payload["display_name"] = toolParams.DisplayName
	}
	if len(toolParams.ProjectBranchOverrides) > 0 {
		payload["project_branch_overrides"] = toolParams.ProjectBranchOverrides
	}
	if len(toolParams.BuildOnCommit) > 0 && string(toolParams.BuildOnCommit) != "null" {
		var buildOn any
		if err := json.Unmarshal(toolParams.BuildOnCommit, &buildOn); err != nil {
			return "", errors.ValidationError("deploy_detached", "build_on_commit", err.Error())
		}
		payload["build_on_commit"] = buildOn
	}

	body, err := t.client.Requester.Do(
		http.MethodPost,
		uri.CreateResourceURI("", "application-build", toolParams.ApplicationBuildID, "detached-app-build", t.orgParams()),
		"application/json",
		payload,
	)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (t *ExtendedTool) executeUpdateBranches(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		Projects      []struct {
			RepoName string `json:"repo_name"`
			Branch   string `json:"branch"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("update_branches", "parameters", err.Error())
	}
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("update_branches", "environment_id", err.Error())
	}
	if len(toolParams.Projects) == 0 {
		return "", errors.ValidationError("update_branches", "projects", "projects array is required and must list every repo")
	}

	projects := make([]map[string]string, 0, len(toolParams.Projects))
	for i, p := range toolParams.Projects {
		if p.RepoName == "" || p.Branch == "" {
			return "", errors.ValidationError("update_branches", "projects", fmt.Sprintf("projects[%d] requires repo_name and branch", i))
		}
		projects = append(projects, map[string]string{"repo_name": p.RepoName, "branch": p.Branch})
	}

	payload := map[string]any{
		"data": map[string]any{
			"type": "application",
			"id":   toolParams.EnvironmentID,
			"attributes": map[string]any{
				"projects": projects,
			},
		},
	}

	body, err := t.client.Requester.Do(
		http.MethodPatch,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "", t.orgParams()),
		"application/json",
		payload,
	)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
