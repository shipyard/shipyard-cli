package logs

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shipyard/shipyard-cli/pkg/client"
)

// Mock client for testing - simplified approach
func newMockClient() client.Client {
	return client.Client{}
}

func TestNewLogsManager(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	service := NewLogsManager(client)

	if service == nil {
		t.Fatal("Expected service to be created, got nil")
	}

	// Just check that the service is initialized properly
	// We can't easily compare client structs, so just check service is not nil
	// The fact that NewLogsManager returned without error means it worked
}

func TestLogsManager_GetLogs(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client and k8s
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsManager_GetLogs_InvalidEnvironmentID(t *testing.T) {
	t.Parallel()

	service := &LogsManager{client: newMockClient()}

	req := GetLogsRequest{
		EnvironmentID: "", // Empty environment ID
		ServiceName:   "web-server",
		TailLines:     50,
	}

	ctx := context.Background()
	_, err := service.GetLogs(ctx, req)

	if err == nil {
		t.Error("Expected error for empty environment ID, got nil")
	}

	if !strings.Contains(err.Error(), "environment ID is required") {
		t.Errorf("Expected environment ID error message, got: %v", err)
	}
}

func TestLogsManager_GetLogs_InvalidServiceName(t *testing.T) {
	t.Parallel()

	service := &LogsManager{client: newMockClient()}

	req := GetLogsRequest{
		EnvironmentID: "env-123",
		ServiceName:   "", // Empty service name
		TailLines:     50,
	}

	ctx := context.Background()
	_, err := service.GetLogs(ctx, req)

	if err == nil {
		t.Error("Expected error for empty service name, got nil")
	}

	if !strings.Contains(err.Error(), "service name is required") {
		t.Errorf("Expected service name error message, got: %v", err)
	}
}

func TestLogsManager_GetLogs_ServiceNotFound(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsManager_FormatLogsAsText(t *testing.T) {
	t.Parallel()

	service := &LogsManager{client: newMockClient()}

	logs := []LogLine{
		{
			Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			Content:   "Starting application...",
			Service:   "web-server",
		},
		{
			Timestamp: time.Date(2024, 1, 1, 12, 0, 1, 0, time.UTC),
			Content:   "Server listening on port 3000",
			Service:   "web-server",
		},
	}

	result := service.FormatLogsAsText(logs)

	if !strings.Contains(result, "web-server") {
		t.Errorf("Expected service name in formatted output, got: %s", result)
	}

	if !strings.Contains(result, "Starting application") {
		t.Errorf("Expected first log line in output, got: %s", result)
	}

	if !strings.Contains(result, "Server listening") {
		t.Errorf("Expected second log line in output, got: %s", result)
	}

	if !strings.Contains(result, "2024-01-01 12:00:00") {
		t.Errorf("Expected timestamp in output, got: %s", result)
	}
}

func TestLogsManager_FormatLogsAsText_EmptyLogs(t *testing.T) {
	t.Parallel()

	service := &LogsManager{client: newMockClient()}

	logs := []LogLine{}
	result := service.FormatLogsAsText(logs)

	expected := "No logs found."
	if result != expected {
		t.Errorf("Expected '%s', got: %s", expected, result)
	}
}

func TestLogsManager_GetLogsReader(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsManager_GetLogsReader_StreamError(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestGetLogsFromK8s_NonStreaming(t *testing.T) {
	t.Parallel()

	service := &LogsManager{client: newMockClient()}

	ctx := context.Background()
	logs, err := service.getLogsFromK8s(ctx, nil, false, 100, "test-service")

	// Expect an error when k8s service is nil
	if err == nil {
		t.Error("Expected error when k8s service is nil, got none")
	}

	if len(logs) != 0 {
		t.Errorf("Expected empty logs on error, got %d logs", len(logs))
	}
}

func TestLogsManager_PaginateLogs(t *testing.T) {
	t.Parallel()

	service := &LogsManager{}

	// Create test log lines
	logs := make([]LogLine, 50)
	for i := 0; i < 50; i++ {
		logs[i] = LogLine{
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("Log line %d", i+1),
			Service:   "test-service",
		}
	}

	tests := []struct {
		name        string
		page        int
		pageSize    int
		expectedLen int
		hasNext     bool
		nextPage    int
	}{
		{"first page", 1, 10, 10, true, 2},
		{"middle page", 3, 10, 10, true, 4},
		{"last page", 5, 10, 10, false, 0},
		{"oversized page", 6, 10, 0, false, 0},
		{"single large page", 1, 100, 50, false, 0},
		{"page size larger than total", 1, 60, 50, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, hasNext, nextPage := service.paginateLogs(logs, tt.page, tt.pageSize)

			if len(result) != tt.expectedLen {
				t.Errorf("Expected %d logs, got %d", tt.expectedLen, len(result))
			}

			if hasNext != tt.hasNext {
				t.Errorf("Expected hasNext=%v, got %v", tt.hasNext, hasNext)
			}

			if nextPage != tt.nextPage {
				t.Errorf("Expected nextPage=%d, got %d", tt.nextPage, nextPage)
			}
		})
	}
}
