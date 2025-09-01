package transport

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// Stdio Transport Tests
func TestNewStdioTransport(t *testing.T) {
	transport := NewStdioTransport()

	if transport == nil {
		t.Fatal("Expected transport to be created, got nil")
	}
	if transport.reader == nil {
		t.Error("Expected reader to be initialized")
	}
	if transport.writer == nil {
		t.Error("Expected writer to be initialized")
	}
}

func TestStdioTransport_Start(t *testing.T) {
	transport := NewStdioTransport()
	ctx := context.Background()

	err := transport.Start(ctx)
	if err != nil {
		t.Errorf("Expected no error starting transport, got: %v", err)
	}

	if transport.ctx == nil {
		t.Error("Expected context to be set after start")
	}
	if transport.cancel == nil {
		t.Error("Expected cancel function to be set after start")
	}
}

func TestStdioTransport_Stop(t *testing.T) {
	transport := NewStdioTransport()
	ctx := context.Background()

	// Start transport first
	err := transport.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transport: %v", err)
	}

	// Stop transport
	err = transport.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping transport, got: %v", err)
	}

	// Verify context is cancelled
	select {
	case <-transport.ctx.Done():
		// Expected
	default:
		t.Error("Expected context to be cancelled after stop")
	}
}

func TestStdioTransport_ReadMessage(t *testing.T) {
	// Create a custom reader with test data
	testInput := "test message\n"
	reader := bufio.NewReader(strings.NewReader(testInput))

	transport := &StdioTransport{
		reader: reader,
		writer: bufio.NewWriter(&bytes.Buffer{}),
	}

	ctx := context.Background()
	transport.Start(ctx)

	message, err := transport.ReadMessage()
	if err != nil {
		t.Errorf("Expected no error reading message, got: %v", err)
	}
	if string(message) != "test message" {
		t.Errorf("Expected 'test message', got '%s'", string(message))
	}
}

func TestStdioTransport_WriteMessage(t *testing.T) {
	var output bytes.Buffer
	writer := bufio.NewWriter(&output)

	transport := &StdioTransport{
		reader: bufio.NewReader(strings.NewReader("")),
		writer: writer,
	}

	ctx := context.Background()
	transport.Start(ctx)

	testMessage := []byte("test response")
	err := transport.WriteMessage(testMessage)
	if err != nil {
		t.Errorf("Expected no error writing message, got: %v", err)
	}

	expected := "test response\n"
	if output.String() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, output.String())
	}
}

func TestStdioTransport_ReadMessage_InvalidJSON(t *testing.T) {
	// Test with malformed input (not JSON but should still read the line)
	testInput := "invalid json content\n"
	reader := bufio.NewReader(strings.NewReader(testInput))

	transport := &StdioTransport{
		reader: reader,
		writer: bufio.NewWriter(&bytes.Buffer{}),
	}

	ctx := context.Background()
	transport.Start(ctx)

	message, err := transport.ReadMessage()
	if err != nil {
		t.Errorf("Expected no error reading invalid JSON, got: %v", err)
	}
	if string(message) != "invalid json content" {
		t.Errorf("Expected 'invalid json content', got '%s'", string(message))
	}
}

func TestStdioTransport_ReadMessage_EOF(t *testing.T) {
	// Create reader that immediately returns EOF
	reader := bufio.NewReader(strings.NewReader(""))

	transport := &StdioTransport{
		reader: reader,
		writer: bufio.NewWriter(&bytes.Buffer{}),
	}

	ctx := context.Background()
	transport.Start(ctx)

	_, err := transport.ReadMessage()
	if err == nil {
		t.Error("Expected error when reading EOF")
	}
	if !strings.Contains(err.Error(), "stdin closed") {
		t.Errorf("Expected 'stdin closed' error, got: %v", err)
	}
}

func TestStdioTransport_WriteMessage_BrokenPipe(t *testing.T) {
	// Create a writer that fails
	failingWriter := &failingWriter{}
	writer := bufio.NewWriter(failingWriter)

	transport := &StdioTransport{
		reader: bufio.NewReader(strings.NewReader("")),
		writer: writer,
	}

	ctx := context.Background()
	transport.Start(ctx)

	testMessage := []byte("test message")
	err := transport.WriteMessage(testMessage)
	if err == nil {
		t.Error("Expected error when writing to broken pipe")
	}
}

// Helper type for testing write failures
type failingWriter struct{}

func (fw *failingWriter) Write(p []byte) (n int, err error) {
	return 0, io.ErrClosedPipe
}

func TestStdioTransport_MessageFraming(t *testing.T) {
	// Test multiple messages separated by newlines
	testInput := "message1\nmessage2\nmessage3\n"
	reader := bufio.NewReader(strings.NewReader(testInput))

	transport := &StdioTransport{
		reader: reader,
		writer: bufio.NewWriter(&bytes.Buffer{}),
	}

	ctx := context.Background()
	transport.Start(ctx)

	// Read first message
	msg1, err := transport.ReadMessage()
	if err != nil {
		t.Errorf("Error reading first message: %v", err)
	}
	if string(msg1) != "message1" {
		t.Errorf("Expected 'message1', got '%s'", string(msg1))
	}

	// Read second message
	msg2, err := transport.ReadMessage()
	if err != nil {
		t.Errorf("Error reading second message: %v", err)
	}
	if string(msg2) != "message2" {
		t.Errorf("Expected 'message2', got '%s'", string(msg2))
	}

	// Read third message
	msg3, err := transport.ReadMessage()
	if err != nil {
		t.Errorf("Error reading third message: %v", err)
	}
	if string(msg3) != "message3" {
		t.Errorf("Expected 'message3', got '%s'", string(msg3))
	}
}

func TestStdioTransport_LargeMessage(t *testing.T) {
	// Create a large message (larger than typical buffer size)
	largeMessage := strings.Repeat("x", 10000) + "\n"
	reader := bufio.NewReader(strings.NewReader(largeMessage))

	transport := &StdioTransport{
		reader: reader,
		writer: bufio.NewWriter(&bytes.Buffer{}),
	}

	ctx := context.Background()
	transport.Start(ctx)

	message, err := transport.ReadMessage()
	if err != nil {
		t.Errorf("Expected no error reading large message, got: %v", err)
	}
	expectedMessage := strings.Repeat("x", 10000)
	if string(message) != expectedMessage {
		t.Errorf("Large message not read correctly, got length %d, expected %d", len(message), len(expectedMessage))
	}
}

func TestStdioTransport_ConcurrentReadWrite(t *testing.T) {
	var output bytes.Buffer
	testInput := "concurrent test\n"

	transport := &StdioTransport{
		reader: bufio.NewReader(strings.NewReader(testInput)),
		writer: bufio.NewWriter(&output),
	}

	ctx := context.Background()
	transport.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(2)

	// Concurrent read
	go func() {
		defer wg.Done()
		message, err := transport.ReadMessage()
		if err != nil {
			t.Errorf("Error in concurrent read: %v", err)
		}
		if string(message) != "concurrent test" {
			t.Errorf("Expected 'concurrent test', got '%s'", string(message))
		}
	}()

	// Concurrent write
	go func() {
		defer wg.Done()
		err := transport.WriteMessage([]byte("concurrent response"))
		if err != nil {
			t.Errorf("Error in concurrent write: %v", err)
		}
	}()

	// Wait for both operations to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for concurrent operations")
	}

	if !strings.Contains(output.String(), "concurrent response") {
		t.Error("Expected concurrent write to succeed")
	}
}

func TestStdioTransport_Cleanup(t *testing.T) {
	transport := NewStdioTransport()
	ctx := context.Background()

	// Start transport
	err := transport.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transport: %v", err)
	}

	// Verify resources are initialized
	if transport.ctx == nil {
		t.Error("Expected context to be set")
	}
	if transport.cancel == nil {
		t.Error("Expected cancel function to be set")
	}

	// Stop transport
	err = transport.Stop()
	if err != nil {
		t.Errorf("Error during cleanup: %v", err)
	}

	// Verify cleanup occurred
	select {
	case <-transport.ctx.Done():
		// Context should be cancelled
	default:
		t.Error("Expected context to be cancelled during cleanup")
	}

	// Test stopping already stopped transport
	err = transport.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping already stopped transport, got: %v", err)
	}
}
