package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/k8s"
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
		Description: "Get live logs from a service in a running environment. previous=true returns the logs of the crashed container's last run before it restarted (an init container or sidecar if that is what crashed), with its restart count, exit code and termination reason. For a stopped environment or a failed build, use get_build_logs",
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
		Previous      bool   `json:"previous,omitempty"`
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
		Previous:      toolParams.Previous,
	}

	// Get logs
	resp, err := t.logsService.GetLogs(ctx, req)
	if err != nil {
		log.Printf("MCP get_logs error: %v", err)
		return "", errors.ParseHTTPError("get_logs", err, toolParams.EnvironmentID)
	}

	return t.formatLogsResponse(resp, toolParams.EnvironmentID, toolParams.ServiceName, toolParams.Page), nil
}

// formatLogsResponse renders a logs response for AI consumption, headed by the container's state for previous-run requests.
func (t *LogsTool) formatLogsResponse(resp *logs.LogsResponse, environmentID, serviceName string, page int) string {
	result := ""
	if resp.State != nil {
		result = formatContainerState(resp.State)
		switch {
		case resp.Run == logs.RunGone:
			return result + fmt.Sprintf("The container restarted, but Kubernetes no longer keeps the logs of its previous run. "+
				"For the crash logs Shipyard stored, call get_build_logs(environment_id=%q, kind=\"crash\", service_name=%q).",
				environmentID, serviceName)
		case resp.Run == logs.RunCurrent && len(resp.Lines) == 0:
			return result + "The container has not restarted, so it has no previous run, and its current run has written no output."
		case resp.Run == logs.RunCurrent:
			result += "The container has not restarted, so it has no previous run. This is its current run:\n"
		case len(resp.Lines) == 0 && page > 1:
			return result + fmt.Sprintf("No more lines on page %d.", page)
		case len(resp.Lines) == 0:
			return result + "No logs from the previous container run."
		}
	} else if len(resp.Lines) == 0 {
		return fmt.Sprintf("No logs found for service %s in environment %s", serviceName, environmentID)
	}

	result += t.logsService.FormatLogsAsText(resp.Lines)
	result += fmt.Sprintf("\nShowing %d log lines for service %s (page %d)", len(resp.Lines), serviceName, page)

	if resp.HasNext {
		result += fmt.Sprintf("\nMore logs available on page %d", resp.NextPage)
	}

	return result
}

// formatContainerState is the header for previous-run logs: which pod, how often it restarted and why it stopped.
func formatContainerState(state *k8s.ContainerState) string {
	line := "Pod " + state.Pod
	switch {
	case state.Init:
		line += " init container " + state.Container
	case state.Container != "":
		line += " container " + state.Container
	}
	line += fmt.Sprintf(": ready=%t, restarts=%d", state.Ready, state.RestartCount)
	if state.Reason != "" {
		line += ", last stopped: " + state.Reason
	}
	if state.ExitCode != nil {
		line += fmt.Sprintf(" (exit code %d)", *state.ExitCode)
	}
	return line + "\n"
}
