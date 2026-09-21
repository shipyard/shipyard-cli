package tools

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/k8s"
	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/schemas"
	"github.com/shipyard/shipyard-cli/pkg/mcp/validation"
	"github.com/shipyard/shipyard-cli/pkg/types"
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

const (
	// execTimeout bounds a single exec. Long enough for a migration check or a
	// test command, short enough that a hung process does not hold the client.
	execTimeout = 60 * time.Second

	// execMaxOutputBytes caps each of stdout and stderr. The result goes into a
	// model's context, so `cat` on a log file has to be truncated, not relayed.
	execMaxOutputBytes = 64 * 1024
)

// podExecutor runs one command in a service's pod. k8s.Service implements it;
// tests supply their own so they need no cluster.
type podExecutor interface {
	ExecCapture(ctx context.Context, command []string, maxBytes int) (k8s.ExecOutput, error)
}

// ServiceTool handles service-related MCP operations
type ServiceTool struct {
	client    client.Client
	name      string
	allowExec bool

	// newExecutor resolves the service to a pod and returns something that can
	// run a command in it. Swapped in tests.
	newExecutor func(c client.Client, envID string, svc *types.Service) (podExecutor, error)
}

// NewServiceTool creates a new service tool. Exec stays disabled: use
// NewServiceToolWithExec for the exec tool itself.
func NewServiceTool(client client.Client, name string) *ServiceTool {
	return NewServiceToolWithExec(client, name, false)
}

// NewServiceToolWithExec creates a service tool, saying whether running commands
// in a customer's containers is permitted. Off unless the operator turned it on:
// every other tool here reads or restarts something Shipyard owns, while this one
// runs arbitrary code inside a running container.
func NewServiceToolWithExec(apiClient client.Client, name string, allowExec bool) *ServiceTool {
	return &ServiceTool{
		client:    apiClient,
		name:      name,
		allowExec: allowExec,
		newExecutor: func(c client.Client, envID string, svc *types.Service) (podExecutor, error) {
			return k8s.New(c, envID, svc)
		},
	}
}

// Definition returns the tool definition for MCP
func (t *ServiceTool) Definition() ToolDefinition {
	if def, exists := serviceToolDefinitions[t.name]; exists {
		// What the model is told has to match what the tool will do, or it
		// spends turns calling something that only ever answers with advice.
		if t.name == "exec_service" && t.allowExec {
			def.Description = "Run a non-interactive command in a service container and return its " +
				"stdout, stderr and exit code. There is no terminal: interactive programs such as " +
				"'bash' or 'vim' will not work, and stdin is not attached. Commands are cut off after " +
				"60 seconds and output is truncated past 64KB."
		}

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
		return t.executeExecService(ctx, params)
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

func (t *ServiceTool) executeExecService(ctx context.Context, params json.RawMessage) (string, error) {
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
		return "", errors.ValidationError("exec_service", "command", "command is required. Example: ['ls', '-la'] or ['cat', '/app/config.json']")
	}

	cliCommand := fmt.Sprintf("shipyard exec --env %s --service %s -- %s",
		toolParams.EnvironmentID, toolParams.ServiceName, strings.Join(toolParams.Command, " "))

	if !t.allowExec {
		return fmt.Sprintf("Running commands in containers is disabled for this MCP server. "+
			"Enable it by setting 'mcp.allow_exec: true' in ~/.shipyard/config.yaml, or "+
			"SHIPYARD_MCP_ALLOW_EXEC=true in the MCP client's environment, then restart the client.\n\n"+
			"To run it yourself now:\n\n%s", cliCommand), nil
	}

	svc, err := t.client.FindService(toolParams.ServiceName, toolParams.EnvironmentID)
	if err != nil {
		log.Printf("MCP exec_service error resolving service: %v", err)
		return "", errors.ParseHTTPError("exec_service", err, toolParams.EnvironmentID)
	}

	executor, err := t.newExecutor(t.client, toolParams.EnvironmentID, svc)
	if err != nil {
		log.Printf("MCP exec_service error connecting to the pod: %v", err)

		// A structured error so the reason survives: the server only runs its
		// substring-matching mapper over plain errors, and "kubeconfig not
		// found" there comes back to the agent as a missing service, which
		// sends it off calling get_services for a service that does exist.
		return "", errors.NewMCPError("exec_service",
			fmt.Sprintf("cannot reach service %q: %v", toolParams.ServiceName, err), err).
			WithSuggestion("The environment may not expose a kubeconfig, or may not be running. " +
				"Check that it is ready, and that 'shipyard exec' works against it from a terminal")
	}

	execCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	out, err := executor.ExecCapture(execCtx, toolParams.Command, execMaxOutputBytes)
	if err != nil {
		// A timeout still carries whatever the command printed first, which is
		// usually the useful part.
		if goerrors.Is(err, context.DeadlineExceeded) {
			return formatExecResult(toolParams.EnvironmentID, toolParams.ServiceName, toolParams.Command, out,
				fmt.Sprintf("timed out after %s and was cut off", execTimeout))
		}

		log.Printf("MCP exec_service error: %v", err)

		return "", errors.NewMCPError("exec_service",
			fmt.Sprintf("command failed to run in service %q: %v", toolParams.ServiceName, err), err).
			WithSuggestion("This is the exec stream failing, not the command exiting non-zero. " +
				"Check that the container is running and that the binary exists in it")
	}

	return formatExecResult(toolParams.EnvironmentID, toolParams.ServiceName, toolParams.Command, out, "")
}

// formatExecResult renders one exec as JSON, the shape the other tools use.
func formatExecResult(envID, serviceName string, command []string, out k8s.ExecOutput, note string) (string, error) {
	response := map[string]interface{}{
		"environment_id": envID,
		"service_name":   serviceName,
		"command":        command,
		"exit_code":      out.ExitCode,
		"stdout":         out.Stdout,
		"stderr":         out.Stderr,
	}

	if out.Truncated {
		response["truncated"] = true
		response["truncated_at_bytes"] = execMaxOutputBytes
	}

	if note != "" {
		response["note"] = note
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal exec response: %w", err)
	}

	return string(jsonData), nil
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
