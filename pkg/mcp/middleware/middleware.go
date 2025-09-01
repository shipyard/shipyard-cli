package middleware

import (
	"encoding/json"
)

// JSONRPCRequest represents a JSON-RPC 2.0 request for middleware processing
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Middleware interface for processing requests
type Middleware interface {
	Process(req *JSONRPCRequest) error
}
