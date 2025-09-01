package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/middleware"
	"github.com/shipyard/shipyard-cli/pkg/mcp/resources"
	"github.com/shipyard/shipyard-cli/pkg/mcp/tools"
	"github.com/shipyard/shipyard-cli/pkg/mcp/transport"
	"github.com/spf13/viper"
)

// JSON-RPC 2.0 structures
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

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
}

// MCP Server
type MCPServer struct {
	config     MCPServerConfig
	transport  transport.Transport
	client     client.Client
	tools      map[string]tools.Tool
	resources  []resources.Resource
	middleware []middleware.Middleware
	running    bool
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

// Create new MCP server
func NewMCPServer(config MCPServerConfig, client client.Client) *MCPServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &MCPServer{
		config:     config,
		client:     client,
		tools:      make(map[string]tools.Tool),
		resources:  make([]resources.Resource, 0),
		middleware: make([]middleware.Middleware, 0),
		ctx:        ctx,
		cancel:     cancel,
	}
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
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			msg, err := s.transport.ReadMessage()
			if err != nil {
				log.Printf("Error reading message: %v", err)
				// If we get EOF or stdin is closed, stop the server
				if err.Error() == "stdin closed" || err == io.EOF {
					log.Println("Input stream closed, stopping server")
					return
				}
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

	// Handle MCP methods
	switch req.Method {
	case "initialize":
		return s.handleInitialize(&req)
	case "tools/list":
		return s.handleListTools(&req)
	case "tools/call":
		return s.handleCallTool(&req)
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
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools":     map[string]interface{}{},
			"resources": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "shipyard-mcp-server",
			"version": "1.0.0",
		},
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
	log.Printf("DEBUG: Registering MCP tools with updated binary")
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
	s.tools["exec_service"] = tools.NewServiceTool(s.client, "exec_service")
	s.tools["port_forward"] = tools.NewServiceTool(s.client, "port_forward")

	// Register volume tools
	s.tools["get_volumes"] = tools.NewVolumeTool(s.client, "get_volumes")
	s.tools["get_snapshots"] = tools.NewVolumeTool(s.client, "get_snapshots")
	s.tools["reset_volume"] = tools.NewVolumeTool(s.client, "reset_volume")
	s.tools["create_snapshot"] = tools.NewVolumeTool(s.client, "create_snapshot")
	s.tools["load_snapshot"] = tools.NewVolumeTool(s.client, "load_snapshot")

	// Register telepresence tools
	s.tools["telepresence_connect"] = tools.NewTelepresenceTool(s.client, "telepresence_connect")

	log.Printf("DEBUG: Registered %d MCP tools", len(s.tools))
}

// Register available resources
func (s *MCPServer) registerResources() {
	log.Printf("DEBUG: Registering MCP resources")
	// Register logs resource
	s.resources = append(s.resources, resources.NewLogsResource(s.client))
	log.Printf("DEBUG: Registered %d MCP resources", len(s.resources))
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
	viper.UnmarshalKey("mcp", &config)

	return config
}
