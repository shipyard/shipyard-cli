package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/middleware"
	"github.com/shipyard/shipyard-cli/pkg/mcp/prompts"
	"github.com/shipyard/shipyard-cli/pkg/mcp/resources"
	"github.com/shipyard/shipyard-cli/pkg/mcp/tools"
	"github.com/shipyard/shipyard-cli/pkg/mcp/transport"
	"github.com/shipyard/shipyard-cli/version"
	"github.com/spf13/viper"
)

// JSON-RPC 2.0 structures
type JSONRPCRequest = middleware.JSONRPCRequest

type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP Server configuration
type MCPServerConfig struct {
	Transport    string `yaml:"transport" mapstructure:"transport"`
	Port         int    `yaml:"port" mapstructure:"port"`
	AuditLogging bool   `yaml:"audit_logging" mapstructure:"audit_logging"`

	// AllowExec lets exec_service run commands inside a customer's containers.
	// Off by default: it is the one tool here that executes arbitrary code in a
	// running environment, so it is the operator's call, not the assistant's.
	AllowExec bool `yaml:"allow_exec" mapstructure:"allow_exec"`
}

// MCP Server
type MCPServer struct {
	config     MCPServerConfig
	transport  transport.Transport
	client     client.Client
	tools      map[string]tools.Tool
	resources  []resources.Resource
	prompts    map[string]prompts.Prompt
	middleware []middleware.Middleware
	running    bool
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	doneOnce   sync.Once
}

// Create new MCP server
func NewMCPServer(config MCPServerConfig, client client.Client) *MCPServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &MCPServer{
		config:     config,
		client:     client,
		tools:      make(map[string]tools.Tool),
		resources:  make([]resources.Resource, 0),
		prompts:    make(map[string]prompts.Prompt),
		middleware: make([]middleware.Middleware, 0),
		ctx:        ctx,
		cancel:     cancel,
		done:       make(chan struct{}),
	}
}

// Done is closed once the server stops handling messages, which happens when
// the client closes the input stream. Callers wait on it so that a disconnected
// client ends the process instead of leaving it running with nothing to read.
func (s *MCPServer) Done() <-chan struct{} {
	return s.done
}

// Start the MCP server
func (s *MCPServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	// Initialize transport based on config
	switch s.config.Transport {
	case "stdio":
		s.transport = transport.NewStdioTransport()
	default:
		return fmt.Errorf("unsupported transport: %s", s.config.Transport)
	}

	// Register tools
	s.registerTools()

	// Register resources
	s.registerResources()

	// Register prompts
	s.registerPrompts()

	// Setup middleware
	s.setupMiddleware()

	// Start transport
	if err := s.transport.Start(s.ctx); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	s.running = true
	log.Printf("MCP server started with %s transport", s.config.Transport)

	// Start message handling loop
	go s.handleMessages()

	return nil
}

// Stop the MCP server
func (s *MCPServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.cancel()
	if s.transport != nil {
		if err := s.transport.Stop(); err != nil {
			log.Printf("Error stopping transport: %v", err)
		}
	}

	s.running = false
	log.Println("MCP server stopped")
	return nil
}

// Check if server is running
func (s *MCPServer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Handle MCP messages
func (s *MCPServer) handleMessages() {
	defer s.doneOnce.Do(func() { close(s.done) })

	for {
		msg, err := s.transport.ReadMessage()
		if err != nil {
			// Check if it's a context cancellation (normal shutdown)
			if err == context.Canceled || err == context.DeadlineExceeded {
				log.Println("Server shutting down")
				return
			}
			// Check if stdin was closed
			if err.Error() == "stdin closed" || err == io.EOF {
				log.Println("Input stream closed, stopping server")
				return
			}
			log.Printf("Error reading message: %v", err)
			continue
		}

		response := s.processMessage(msg)
		if response != nil {
			if err := s.transport.WriteMessage(response); err != nil {
				log.Printf("Error writing response: %v", err)
			}
		}
	}
}

// Process individual JSON-RPC message
func (s *MCPServer) processMessage(data []byte) []byte {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return s.errorResponse(nil, -32700, "Parse error", nil)
	}

	if err := s.validateRequest(&req); err != nil {
		return s.errorResponse(req.ID, -32600, "Invalid Request", err.Error())
	}

	// Apply middleware
	for _, mw := range s.middleware {
		middlewareReq := &middleware.JSONRPCRequest{
			JSONRPC: req.JSONRPC,
			ID:      req.ID,
			Method:  req.Method,
			Params:  req.Params,
		}
		if err := mw.Process(middlewareReq); err != nil {
			return s.errorResponse(req.ID, -32000, "Middleware error", err.Error())
		}
	}

	// Notifications carry no id and must never be answered, not even with an
	// error. Clients send lifecycle ones this server does not act on, such as
	// notifications/initialized and notifications/cancelled.
	if strings.HasPrefix(req.Method, "notifications/") {
		return nil
	}

	// Handle MCP methods
	switch req.Method {
	case "initialize":
		return s.handleInitialize(&req)
	case "ping":
		// The spec's ping utility: answer promptly with an empty result. A
		// client that uses ping as a health check treats an error reply as a
		// dead server and drops the connection.
		return s.successResponse(req.ID, map[string]interface{}{})
	case "tools/list":
		return s.handleListTools(&req)
	case "tools/call":
		return s.handleCallTool(&req)
	case "prompts/list":
		return s.handleListPrompts(&req)
	case "prompts/get":
		return s.handleGetPrompt(&req)
	case "resources/list":
		return s.handleListResources(&req)
	case "resources/read":
		return s.handleReadResource(&req)
	default:
		return s.errorResponse(req.ID, -32601, "Method not found", nil)
	}
}

// Handle initialize request
func (s *MCPServer) handleInitialize(req *JSONRPCRequest) []byte {
	// The client states which MCP revision it wants. Echo it back when this
	// server speaks it, otherwise answer with the newest one it does.
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}

	if len(req.Params) > 0 {
		// A malformed params object is not fatal here: an empty requested
		// version negotiates to the latest, which is what the client gets.
		if err := json.Unmarshal(req.Params, &params); err != nil {
			log.Printf("MCP initialize: could not read requested protocolVersion: %v", err)
		}
	}

	result := map[string]interface{}{
		"protocolVersion": negotiateProtocolVersion(params.ProtocolVersion),
		"capabilities": map[string]interface{}{
			"tools":     map[string]interface{}{},
			"resources": map[string]interface{}{},
			"prompts":   map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "shipyard-mcp-server",
			"version": version.Version,
		},
		"instructions": Instructions(),
	}

	return s.successResponse(req.ID, result)
}

// Handle list tools request
func (s *MCPServer) handleListTools(req *JSONRPCRequest) []byte {
	toolsList := make([]interface{}, 0, len(s.tools))
	for _, tool := range s.tools {
		toolsList = append(toolsList, tool.Definition())
	}

	result := map[string]interface{}{
		"tools": toolsList,
	}

	return s.successResponse(req.ID, result)
}

// Handle list prompts request
func (s *MCPServer) handleListPrompts(req *JSONRPCRequest) []byte {
	promptsList := make([]interface{}, 0, len(s.prompts))
	for _, prompt := range s.prompts {
		promptsList = append(promptsList, prompt.Definition())
	}

	result := map[string]interface{}{
		"prompts": promptsList,
	}

	return s.successResponse(req.ID, result)
}

// Handle get prompt request
func (s *MCPServer) handleGetPrompt(req *JSONRPCRequest) []byte {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments,omitempty"`
	}

	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return s.errorResponse(req.ID, -32602, "Invalid params", err.Error())
		}
	}

	if params.Name == "" {
		return s.errorResponse(req.ID, -32602, "Invalid params", "prompt name is required")
	}

	prompt, ok := s.prompts[params.Name]
	if !ok {
		return s.errorResponse(req.ID, -32602, "Unknown prompt", params.Name)
	}

	result, err := prompt.Get(params.Arguments)
	if err != nil {
		log.Printf("MCP prompts/get error for %s: %v", params.Name, err)
		return s.errorResponse(req.ID, -32603, "Internal error", err.Error())
	}

	return s.successResponse(req.ID, result)
}

// Handle call tool request
func (s *MCPServer) handleCallTool(req *JSONRPCRequest) []byte {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.errorResponse(req.ID, -32602, "Invalid params", err.Error())
	}

	tool, exists := s.tools[params.Name]
	if !exists {
		return s.errorResponse(req.ID, -32000, "Tool not found", params.Name)
	}

	result, err := tool.Execute(s.ctx, params.Arguments)
	if err != nil {
		log.Printf("MCP server tool execution error for %s: %v", params.Name, err)

		// Check if error is already an MCPError to avoid double-processing
		var mcpErr *errors.MCPError
		if mcpError, ok := err.(*errors.MCPError); ok {
			mcpErr = mcpError
		} else {
			// Use improved error handling for non-MCP errors
			mcpErr = errors.ParseHTTPError(params.Name, err, "")
		}
		return s.errorResponse(req.ID, mcpErr.ToJSONRPCCode(), mcpErr.Error(), nil)
	}

	return s.successResponse(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": result,
			},
		},
	})
}

// Handle list resources request
func (s *MCPServer) handleListResources(req *JSONRPCRequest) []byte {
	resourcesList := make([]interface{}, 0, len(s.resources))
	for _, resource := range s.resources {
		resourcesList = append(resourcesList, resource.Definition())
	}

	result := map[string]interface{}{
		"resources": resourcesList,
	}

	return s.successResponse(req.ID, result)
}

// Handle read resource request
func (s *MCPServer) handleReadResource(req *JSONRPCRequest) []byte {
	var params struct {
		URI string `json:"uri"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.errorResponse(req.ID, -32602, "Invalid params", err.Error())
	}

	if params.URI == "" {
		return s.errorResponse(req.ID, -32602, "Missing URI parameter", nil)
	}

	// Find resource that can handle this URI
	var targetResource resources.Resource
	for _, resource := range s.resources {
		if resource.IsAvailable(s.ctx, params.URI) {
			targetResource = resource
			break
		}
	}

	if targetResource == nil {
		return s.errorResponse(req.ID, -32000, "Resource not found", params.URI)
	}

	// Get resource content
	reader, mimeType, err := targetResource.GetContent(s.ctx, params.URI)
	if err != nil {
		log.Printf("MCP server resource read error for %s: %v", params.URI, err)
		mcpErr := errors.ParseHTTPError("read_resource", err, params.URI)
		return s.errorResponse(req.ID, mcpErr.ToJSONRPCCode(), mcpErr.Error(), nil)
	}

	// Read content from reader
	content, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("MCP server resource content read error for %s: %v", params.URI, err)
		mcpErr := errors.NewMCPError("read_resource", "failed to read resource content", err).
			WithSuggestion("The resource may be corrupted or temporarily unavailable. Please try again")
		return s.errorResponse(req.ID, mcpErr.ToJSONRPCCode(), mcpErr.Error(), nil)
	}

	return s.successResponse(req.ID, map[string]interface{}{
		"contents": []interface{}{
			map[string]interface{}{
				"uri":      params.URI,
				"mimeType": mimeType,
				"text":     string(content),
			},
		},
	})
}

// Validate JSON-RPC request
func (s *MCPServer) validateRequest(req *JSONRPCRequest) error {
	if req.JSONRPC != "2.0" {
		return fmt.Errorf("invalid JSON-RPC version: %s", req.JSONRPC)
	}
	if req.Method == "" {
		return fmt.Errorf("missing method")
	}
	return nil
}

// Register available tools
func (s *MCPServer) registerTools() {
	// Register environment tools
	s.tools["get_environments"] = tools.NewEnvironmentTool(s.client, "get_environments")
	s.tools["get_environment"] = tools.NewEnvironmentTool(s.client, "get_environment")
	s.tools["restart_environment"] = tools.NewEnvironmentTool(s.client, "restart_environment")
	s.tools["stop_environment"] = tools.NewEnvironmentTool(s.client, "stop_environment")
	s.tools["cancel_environment"] = tools.NewEnvironmentTool(s.client, "cancel_environment")
	s.tools["rebuild_environment"] = tools.NewEnvironmentTool(s.client, "rebuild_environment")
	s.tools["revive_environment"] = tools.NewEnvironmentTool(s.client, "revive_environment")

	// Register organization tools
	s.tools["get_orgs"] = tools.NewOrgTool(s.client, "get_orgs")
	s.tools["get_org"] = tools.NewOrgTool(s.client, "get_org")
	s.tools["set_org"] = tools.NewOrgTool(s.client, "set_org")

	// Register logs tool
	s.tools["get_logs"] = tools.NewLogsTool(s.client, "get_logs")

	// Register service tools
	s.tools["get_services"] = tools.NewServiceTool(s.client, "get_services")
	s.tools["exec_service"] = tools.NewServiceToolWithExec(s.client, "exec_service", s.config.AllowExec)
	s.tools["port_forward"] = tools.NewServiceTool(s.client, "port_forward")

	// Register volume tools
	s.tools["get_volumes"] = tools.NewVolumeTool(s.client, "get_volumes")
	s.tools["get_snapshots"] = tools.NewVolumeTool(s.client, "get_snapshots")
	s.tools["reset_volume"] = tools.NewVolumeTool(s.client, "reset_volume")
	s.tools["create_snapshot"] = tools.NewVolumeTool(s.client, "create_snapshot")
	s.tools["load_snapshot"] = tools.NewVolumeTool(s.client, "load_snapshot")

	// Register telepresence tools
	s.tools["telepresence_connect"] = tools.NewTelepresenceTool(s.client, "telepresence_connect")
}

// Register available resources
func (s *MCPServer) registerResources() {
	// Register logs resource
	s.resources = append(s.resources, resources.NewLogsResource(s.client))
}

// Register prompts
func (s *MCPServer) registerPrompts() {
	// Register the verification loop prompt
	verify := prompts.NewVerifyPrompt()
	s.prompts[verify.Definition().Name] = verify
}

// Setup middleware chain
func (s *MCPServer) setupMiddleware() {
}

// Create success response
func (s *MCPServer) successResponse(id interface{}, result interface{}) []byte {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(response)
	return data
}

// Create error response
func (s *MCPServer) errorResponse(id interface{}, code int, message string, data interface{}) []byte {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	responseData, _ := json.Marshal(response)
	return responseData
}

// Load configuration from Viper
func LoadMCPServerConfig() MCPServerConfig {
	var config MCPServerConfig

	// Set defaults
	viper.SetDefault("mcp.transport", "stdio")
	viper.SetDefault("mcp.port", 8080)
	viper.SetDefault("mcp.audit_logging", true)

	// Unmarshal config
	if err := viper.UnmarshalKey("mcp", &config); err != nil {
		log.Printf("MCP config: could not read the mcp section, using defaults: %v", err)
	}

	config.AllowExec = allowExecFromEnv(config.AllowExec)

	return config
}

// allowExecFromEnv applies SHIPYARD_MCP_ALLOW_EXEC over the config file value.
//
// It reads the environment directly instead of binding it into viper. Anything
// viper knows about is written back by viper.WriteConfig, which set_org calls:
// a bound SHIPYARD_MCP_ALLOW_EXEC=true in one client's env block would land in
// ~/.shipyard/config.yaml and turn exec on for every client on the machine,
// permanently. An unparseable value turns exec off rather than guessing.
func allowExecFromEnv(fromFile bool) bool {
	raw, set := os.LookupEnv("SHIPYARD_MCP_ALLOW_EXEC")
	if !set {
		return fromFile
	}

	allow, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		log.Printf("MCP config: SHIPYARD_MCP_ALLOW_EXEC=%q is not true or false; exec stays off", raw)
		return false
	}

	return allow
}
