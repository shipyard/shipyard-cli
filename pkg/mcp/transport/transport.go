package transport

import (
	"context"
)

// Transport interface for MCP communication
type Transport interface {
	Start(ctx context.Context) error
	Stop() error
	// ReadMessage returns the next message. An error is terminal: the input is
	// gone and no later call will return a message, so the caller stops reading.
	ReadMessage() ([]byte, error)
	WriteMessage(data []byte) error
}
