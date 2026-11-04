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
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description"`
	InputSchema interface{}      `json:"inputSchema"`
	Annotations *ToolAnnotations `json:"annotations,omitempty"`
}

// ToolAnnotations are the MCP behavior hints clients use to decide whether a
// tool call needs approval. Every hint is serialized even when false: the spec
// defaults an absent destructiveHint to true and openWorldHint to true, and
// some directories require all hints to be explicit.
type ToolAnnotations struct {
	Title           string `json:"title"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}
