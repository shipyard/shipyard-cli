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
	reader  *bufio.Reader
	writer  *bufio.Writer
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
	msgChan chan []byte
	errChan chan error
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport() *StdioTransport {
	return &StdioTransport{
		reader:  bufio.NewReader(os.Stdin),
		writer:  bufio.NewWriter(os.Stdout),
		msgChan: make(chan []byte, 1),
		errChan: make(chan error, 1),
	}
}

// Start the stdio transport
func (t *StdioTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.msgChan == nil {
		t.msgChan = make(chan []byte, 1)
	}
	if t.errChan == nil {
		t.errChan = make(chan error, 1)
	}

	t.ctx, t.cancel = context.WithCancel(ctx)

	// Start goroutine to read from stdin
	go t.readLoop()

	return nil
}

// readLoop continuously reads from stdin in a separate goroutine
func (t *StdioTransport) readLoop() {
	for {
		line, err := t.reader.ReadBytes('\n')
		if err != nil {
			var sendErr error
			if err == io.EOF {
				sendErr = fmt.Errorf("stdin closed")
			} else {
				sendErr = fmt.Errorf("failed to read from stdin: %w", err)
			}
			select {
			case t.errChan <- sendErr:
			case <-t.ctx.Done():
			}
			return
		}

		// Remove trailing newline
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}

		select {
		case t.msgChan <- line:
		case <-t.ctx.Done():
			return
		}
	}
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
	select {
	case msg := <-t.msgChan:
		return msg, nil
	case err := <-t.errChan:
		return nil, err
	case <-t.ctx.Done():
		return nil, t.ctx.Err()
	}
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
