package tools

import (
	"context"
	"encoding/json"
)

// Tool interface for MCP tools
type Tool interface {
	Definition() ToolDefinition
	Execute(ctx context.Context, params json.RawMessage) (string, error)
}

// ToolDefinition describes an MCP tool
type ToolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}
