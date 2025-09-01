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
	"github.com/shipyard/shipyard-cli/pkg/services/logs"
)

// LogsTool handles log-related MCP operations
type LogsTool struct {
	client      client.Client
	name        string
	logsService *logs.LogsManager
}

// NewLogsTool creates a new logs tool
func NewLogsTool(client client.Client, name string) *LogsTool {
	return &LogsTool{
		client:      client,
		name:        name,
		logsService: logs.NewLogsManager(client),
	}
}

// Definition returns the tool definition for MCP
func (t *LogsTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        t.name,
		Description: "Get logs from a service in an environment",
		InputSchema: schemas.LogsSchema(),
	}
}

// Execute runs the tool with given parameters
func (t *LogsTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	log.Printf("MCP logs tool execution started with params: %s", string(params))

	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		ServiceName   string `json:"service_name"`
		Tail          int64  `json:"tail,omitempty"`
		Page          int    `json:"page,omitempty"`
		PageSize      int    `json:"page_size,omitempty"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("get_logs", "parameters", err.Error())
	}

	// Validate parameters
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("get_logs", "environment_id", err.Error())
	}

	if err := validation.ValidateServiceName(toolParams.ServiceName); err != nil {
		return "", errors.ValidationError("get_logs", "service_name", err.Error())
	}

	if toolParams.Tail == 0 {
		toolParams.Tail = 100
	}

	if err := validation.ValidateLogTail(int(toolParams.Tail)); err != nil {
		return "", errors.ValidationError("get_logs", "tail", err.Error())
	}

	// Set pagination defaults
	if toolParams.Page == 0 {
		toolParams.Page = 1
	}
	if toolParams.PageSize == 0 {
		toolParams.PageSize = 100
	}

	// Validate pagination parameters
	if err := validation.ValidatePagination(toolParams.Page, toolParams.PageSize); err != nil {
		return "", errors.ValidationError("get_logs", "pagination", err.Error())
	}

	// Create logs request
	req := logs.GetLogsRequest{
		EnvironmentID: toolParams.EnvironmentID,
		ServiceName:   toolParams.ServiceName,
		Follow:        false,
		TailLines:     toolParams.Tail,
		Page:          toolParams.Page,
		PageSize:      toolParams.PageSize,
	}

	// Get logs
	resp, err := t.logsService.GetLogs(ctx, req)
	if err != nil {
		log.Printf("MCP get_logs error: %v", err)
		return "", errors.ParseHTTPError("get_logs", err, toolParams.EnvironmentID)
	}

	// Format response for AI consumption
	if len(resp.Lines) == 0 {
		return fmt.Sprintf("No logs found for service %s in environment %s", toolParams.ServiceName, toolParams.EnvironmentID), nil
	}

	result := t.logsService.FormatLogsAsText(resp.Lines)
	result += fmt.Sprintf("\nShowing %d log lines for service %s (page %d)", len(resp.Lines), toolParams.ServiceName, toolParams.Page)

	if resp.HasNext {
		result += fmt.Sprintf("\nMore logs available on page %d", resp.NextPage)
	}

	return result, nil
}
