package logs

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/k8s"
)

// LogsManager handles logs business operations
type LogsManager struct {
	client client.Client
}

// NewLogsManager creates a new logs manager
func NewLogsManager(client client.Client) *LogsManager {
	return &LogsManager{client: client}
}

// GetLogsRequest contains parameters for getting logs
type GetLogsRequest struct {
	EnvironmentID string
	ServiceName   string
	Follow        bool
	TailLines     int64
	Page          int
	PageSize      int
}

// LogLine represents a single log line with metadata
type LogLine struct {
	Timestamp time.Time `json:"timestamp"`
	Content   string    `json:"content"`
	Service   string    `json:"service"`
}

// LogsResponse contains the result of log retrieval
type LogsResponse struct {
	Lines    []LogLine `json:"lines"`
	Service  string    `json:"service"`
	EnvID    string    `json:"environment_id"`
	HasNext  bool      `json:"has_next"`
	NextPage int       `json:"next_page"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

// GetLogs retrieves logs for a service in an environment
func (s *LogsManager) GetLogs(ctx context.Context, req GetLogsRequest) (*LogsResponse, error) {
	if req.EnvironmentID == "" {
		return nil, fmt.Errorf("environment ID is required")
	}
	if req.ServiceName == "" {
		return nil, fmt.Errorf("service name is required")
	}
	if req.TailLines == 0 {
		req.TailLines = 100 // default
	}
	if req.Page == 0 {
		req.Page = 1 // default
	}
	if req.PageSize == 0 {
		req.PageSize = 20 // default
	}

	// Find the service
	svc, err := s.client.FindService(req.ServiceName, req.EnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to find service %s: %w", req.ServiceName, err)
	}

	// Create k8s service for log access
	k8sService, err := k8s.New(s.client, req.EnvironmentID, svc)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s connection: %w", err)
	}

	// Get logs from k8s
	allLogs, err := s.getLogsFromK8s(ctx, k8sService, req.Follow, req.TailLines, req.ServiceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	// Apply pagination to the logs
	paginatedLogs, hasNext, nextPage := s.paginateLogs(allLogs, req.Page, req.PageSize)

	return &LogsResponse{
		Lines:    paginatedLogs,
		Service:  req.ServiceName,
		EnvID:    req.EnvironmentID,
		HasNext:  hasNext,
		NextPage: nextPage,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

// getLogsFromK8s retrieves logs from kubernetes and returns them as LogLine slice
func (s *LogsManager) getLogsFromK8s(ctx context.Context, k8sService *k8s.Service, follow bool, tailLines int64, serviceName string) ([]LogLine, error) {
	// Get raw logs by calling the k8s service directly and capturing output
	logText, err := s.getRawLogsFromK8sService(k8sService, tailLines)
	if err != nil {
		return nil, fmt.Errorf("failed to get raw logs: %w", err)
	}

	// Parse the raw log text into structured LogLine objects
	return s.parseLogTextWithService(logText, serviceName), nil
}

// getRawLogsFromK8sService gets raw log text from the k8s service
func (s *LogsManager) getRawLogsFromK8sService(k8sService *k8s.Service, tailLines int64) (string, error) {
	// We need to replicate the k8s.Service.Logs functionality but capture the output
	// Since we can't easily modify the existing k8s package, we'll create our own k8s client

	// This is a temporary workaround - ideally we'd refactor the k8s package
	// For now, let's try to call the k8s service's Logs method indirectly

	// Since k8s.Service.Logs prints to stdout, we can't easily capture it
	// We need to implement our own k8s logs fetching
	return s.getLogsDirectlyFromK8sAPI(k8sService, tailLines)
}

// getLogsDirectlyFromK8sAPI directly calls the k8s API to get logs
func (s *LogsManager) getLogsDirectlyFromK8sAPI(k8sService *k8s.Service, tailLines int64) (string, error) {
	if k8sService == nil {
		return "", fmt.Errorf("k8s service is nil")
	}
	// Use the new GetLogsAsString method we added to k8s.Service
	return k8sService.GetLogsAsString(false, tailLines)
}

func (s *LogsManager) parseLogText(logText string) []LogLine {
	return s.parseLogTextWithService(logText, "")
}

func (s *LogsManager) parseLogTextWithService(logText, serviceName string) []LogLine {
	if logText == "" {
		return []LogLine{}
	}

	lines := strings.Split(strings.TrimSpace(logText), "\n")
	logLines := make([]LogLine, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		logLines = append(logLines, LogLine{
			Timestamp: time.Now(), // TODO: Parse actual timestamp from log line if available
			Content:   line,
			Service:   serviceName,
		})
	}

	return logLines
}

// FormatLogsAsText formats logs for text display
func (s *LogsManager) FormatLogsAsText(logs []LogLine) string {
	if len(logs) == 0 {
		return "No logs found."
	}

	result := fmt.Sprintf("Logs for service %s:\n\n", logs[0].Service)
	for _, line := range logs {
		result += fmt.Sprintf("[%s] %s\n",
			line.Timestamp.Format("2006-01-02 15:04:05"),
			line.Content)
	}

	return result
}

// GetLogsReader returns an io.Reader for logs (useful for MCP resources)
func (s *LogsManager) GetLogsReader(ctx context.Context, req GetLogsRequest) (io.Reader, error) {
	response, err := s.GetLogs(ctx, req)
	if err != nil {
		return nil, err
	}

	// Format logs as text and return as reader
	logText := s.FormatLogsAsText(response.Lines)
	return strings.NewReader(logText), nil
}

// paginateLogs applies pagination to a slice of log lines
func (s *LogsManager) paginateLogs(logs []LogLine, page, pageSize int) ([]LogLine, bool, int) {
	totalLogs := len(logs)
	if totalLogs == 0 {
		return []LogLine{}, false, 0
	}

	// Calculate pagination bounds
	startIdx := (page - 1) * pageSize
	if startIdx >= totalLogs {
		// Page is beyond available data
		return []LogLine{}, false, 0
	}

	endIdx := startIdx + pageSize
	if endIdx > totalLogs {
		endIdx = totalLogs
	}

	// Get the page slice
	pageSlice := logs[startIdx:endIdx]

	// Determine if there's a next page
	hasNext := endIdx < totalLogs
	var nextPage int
	if hasNext {
		nextPage = page + 1
	}

	return pageSlice, hasNext, nextPage
}
