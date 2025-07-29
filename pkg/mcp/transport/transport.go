package transport

import (
	"context"
)

// Transport interface for MCP communication
type Transport interface {
	Start(ctx context.Context) error
	Stop() error
	ReadMessage() ([]byte, error)
	WriteMessage(data []byte) error
}
