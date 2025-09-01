package transport

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
)

// StdioTransport implements Transport interface for stdio communication
type StdioTransport struct {
	reader *bufio.Reader
	writer *bufio.Writer
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport() *StdioTransport {
	return &StdioTransport{
		reader: bufio.NewReader(os.Stdin),
		writer: bufio.NewWriter(os.Stdout),
	}
}

// Start the stdio transport
func (t *StdioTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.ctx, t.cancel = context.WithCancel(ctx)
	return nil
}

// Stop the stdio transport
func (t *StdioTransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel != nil {
		t.cancel()
	}

	// Flush any pending writes
	if t.writer != nil {
		t.writer.Flush()
	}

	return nil
}

// ReadMessage reads a JSON-RPC message from stdin
func (t *StdioTransport) ReadMessage() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.ctx.Err() != nil {
		return nil, t.ctx.Err()
	}

	// Read line from stdin
	line, err := t.reader.ReadBytes('\n')
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("stdin closed")
		}
		return nil, fmt.Errorf("failed to read from stdin: %w", err)
	}

	// Remove trailing newline
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}

	return line, nil
}

// WriteMessage writes a JSON-RPC message to stdout
func (t *StdioTransport) WriteMessage(data []byte) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.ctx.Err() != nil {
		return t.ctx.Err()
	}

	// Write message with newline
	_, err := t.writer.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write to stdout: %w", err)
	}

	// Flush immediately for real-time communication
	return t.writer.Flush()
}
