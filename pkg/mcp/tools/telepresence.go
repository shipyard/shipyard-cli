package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/k8s"
	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/schemas"
	"github.com/shipyard/shipyard-cli/pkg/mcp/validation"
)

// telepresenceToolDefinitions maps tool names to their definitions
var telepresenceToolDefinitions = map[string]ToolDefinition{
	"telepresence_connect": {
		Name:        "telepresence_connect",
		Description: "Connect to an environment via telepresence",
		InputSchema: schemas.EnvironmentIDSchema(),
	},
}

// TelepresenceTool handles telepresence operations
type TelepresenceTool struct {
	client client.Client
	name   string
}

// NewTelepresenceTool creates a new telepresence tool
func NewTelepresenceTool(client client.Client, name string) *TelepresenceTool {
	return &TelepresenceTool{
		client: client,
		name:   name,
	}
}

// Definition returns the tool definition for MCP
func (t *TelepresenceTool) Definition() ToolDefinition {
	if def, exists := telepresenceToolDefinitions[t.name]; exists {
		return def
	}

	// Fallback for unknown tools
	return ToolDefinition{
		Name:        t.name,
		Description: "Unknown telepresence operation",
		InputSchema: map[string]interface{}{"type": "object"},
	}
}

// Execute runs the tool with given parameters
func (t *TelepresenceTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	log.Printf("MCP tool execution started: %s with params: %s", t.name, string(params))

	switch t.name {
	case "telepresence_connect":
		return t.executeTelepresenceConnect(params)
	default:
		return "", fmt.Errorf("unknown operation: %s", t.name)
	}
}

func (t *TelepresenceTool) executeTelepresenceConnect(params json.RawMessage) (string, error) {
	// Parse parameters
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
	}

	if len(params) > 0 {
		if err := json.Unmarshal(params, &toolParams); err != nil {
			return "", errors.ValidationError("telepresence_connect", "parameters", err.Error())
		}
	}

	// Validate environment ID
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("telepresence_connect", "environment_id", err.Error())
	}

	// Check if telepresence is installed
	if _, err := exec.LookPath("telepresence"); err != nil {
		return "", fmt.Errorf("telepresence not found, please make sure it's in your PATH")
	}

	// Get kubeconfig for the environment
	k, err := k8s.NewConfig(t.client, toolParams.EnvironmentID)
	if err != nil {
		return "", errors.ParseHTTPError("telepresence_connect", err, toolParams.EnvironmentID)
	}

	// Execute telepresence connect command
	cmd := exec.Command(
		"telepresence",
		"connect",
		fmt.Sprintf("--kubeconfig=%s", k.Path),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("telepresence connect failed: %w\nOutput: %s", err, string(output))
	}

	result := fmt.Sprintf("Successfully connected telepresence to environment %s\nOutput: %s", toolParams.EnvironmentID, string(output))
	log.Printf("Telepresence connect completed for environment %s", toolParams.EnvironmentID)
	return result, nil
}
