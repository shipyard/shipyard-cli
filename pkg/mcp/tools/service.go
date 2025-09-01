package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/schemas"
	"github.com/shipyard/shipyard-cli/pkg/mcp/validation"
)

// serviceToolDefinitions maps service tool names to their definitions
var serviceToolDefinitions = map[string]ToolDefinition{
	"get_services": {
		Name:        "get_services",
		Description: "List services in an environment",
		InputSchema: schemas.EnvironmentIDSchema(),
	},
	"exec_service": {
		Name:        "exec_service",
		Description: "Execute commands in service containers",
		InputSchema: schemas.ServiceExecSchema(),
	},
	"port_forward": {
		Name:        "port_forward",
		Description: "Port forward services to local machine",
		InputSchema: schemas.ServicePortForwardSchema(),
	},
}

// ServiceTool handles service-related MCP operations
type ServiceTool struct {
	client client.Client
	name   string
}

// NewServiceTool creates a new service tool
func NewServiceTool(client client.Client, name string) *ServiceTool {
	return &ServiceTool{
		client: client,
		name:   name,
	}
}

// Definition returns the tool definition for MCP
func (t *ServiceTool) Definition() ToolDefinition {
	if def, exists := serviceToolDefinitions[t.name]; exists {
		return def
	}

	// Fallback for unknown tools
	return ToolDefinition{
		Name:        t.name,
		Description: "Unknown service operation",
		InputSchema: map[string]interface{}{"type": "object"},
	}
}

// Execute runs the tool with given parameters
func (t *ServiceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	log.Printf("MCP service tool execution started: %s with params: %s", t.name, string(params))

	switch t.name {
	case "get_services":
		return t.executeGetServices(params)
	case "exec_service":
		return t.executeExecService(params)
	case "port_forward":
		return t.executePortForward(params)
	default:
		return "", fmt.Errorf("unknown service operation: %s", t.name)
	}
}

func (t *ServiceTool) executeGetServices(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("get_services", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("get_services", "environment_id", err.Error())
	}

	// Get services from the client
	services, err := t.client.AllServices(toolParams.EnvironmentID)
	if err != nil {
		log.Printf("MCP get_services error: %v", err)
		return "", errors.ParseHTTPError("get_services", err, toolParams.EnvironmentID)
	}

	// Format response as JSON for consistent API
	response := map[string]interface{}{
		"environment_id": toolParams.EnvironmentID,
		"service_count":  len(services),
		"services":       services,
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal services response: %w", err)
	}

	return string(jsonData), nil
}

func (t *ServiceTool) executeExecService(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string   `json:"environment_id"`
		ServiceName   string   `json:"service_name"`
		Command       []string `json:"command"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("exec_service", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("exec_service", "environment_id", err.Error())
	}

	if toolParams.ServiceName == "" {
		return "", errors.ValidationError("exec_service", "service_name", "service_name is required. Example: 'web', 'api', 'database'")
	}

	if len(toolParams.Command) == 0 {
		return "", errors.ValidationError("exec_service", "command", "command is required. Example: ['ls', '-la'] or ['bash']")
	}

	// Note: exec_service requires interactive session handling which is not suitable for MCP
	// Return information about the limitation
	return fmt.Sprintf("Cannot execute commands interactively via MCP. To execute '%v' in service '%s' of environment '%s', use the CLI command:\n\nshipyard exec --env %s --service %s -- %v",
		toolParams.Command, toolParams.ServiceName, toolParams.EnvironmentID, toolParams.EnvironmentID, toolParams.ServiceName, toolParams.Command), nil
}

func (t *ServiceTool) executePortForward(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string   `json:"environment_id"`
		ServiceName   string   `json:"service_name"`
		Ports         []string `json:"ports"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("port_forward", "parameters", err.Error())
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("port_forward", "environment_id", err.Error())
	}

	if toolParams.ServiceName == "" {
		return "", errors.ValidationError("port_forward", "service_name", "service_name is required. Example: 'web', 'api', 'database'")
	}

	if len(toolParams.Ports) == 0 {
		return "", errors.ValidationError("port_forward", "ports", "ports are required. Example: ['8080:80', '3000:3000']")
	}

	// Note: port_forward requires long-running session handling which is not suitable for MCP
	// Return information about the limitation and CLI command to use
	return fmt.Sprintf("Cannot start port forwarding via MCP as it requires a persistent connection. To port-forward '%v' for service '%s' in environment '%s', use the CLI command:\n\nshipyard port-forward --env %s --service %s --ports %v",
		toolParams.Ports, toolParams.ServiceName, toolParams.EnvironmentID, toolParams.EnvironmentID, toolParams.ServiceName, toolParams.Ports), nil
}
