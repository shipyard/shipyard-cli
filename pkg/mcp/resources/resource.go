package resources

import (
	"context"
	"io"
)

// Resource interface for MCP resources
type Resource interface {
	Definition() ResourceDefinition
	GetContent(ctx context.Context, uri string) (io.Reader, string, error) // Returns reader, mimeType, error
	IsAvailable(ctx context.Context, uri string) bool
}

// ResourceDefinition describes an MCP resource
type ResourceDefinition struct {
	URI         string                 `json:"uri"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	MimeType    string                 `json:"mimeType,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ResourceTemplate represents a resource template that can match URIs
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}
