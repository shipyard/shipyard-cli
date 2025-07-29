package resources

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/services/logs"
)

// LogsResource handles logs as MCP resources
type LogsResource struct {
	client      client.Client
	logsService *logs.LogsManager
	uriPattern  *regexp.Regexp
}

// NewLogsResource creates a new logs resource
func NewLogsResource(client client.Client) *LogsResource {
	return &LogsResource{
		client:      client,
		logsService: logs.NewLogsManager(client),
		uriPattern:  regexp.MustCompile(`^logs://([^/]+)/([^/?]+)(?:\?(.+))?$`),
	}
}

// Definition returns the resource template definition for MCP
func (r *LogsResource) Definition() ResourceDefinition {
	return ResourceDefinition{
		URI:         "logs://{environment_id}/{service_name}",
		Name:        "Service Logs",
		Description: "Get logs from a service in an environment. URI format: logs://{environment_id}/{service_name}?tail=100",
		MimeType:    "text/plain",
		Metadata: map[string]interface{}{
			"parameters": map[string]interface{}{
				"tail": map[string]interface{}{
					"type":        "integer",
					"description": "Number of lines from the end of the logs to show",
					"default":     100,
				},
			},
		},
	}
}

// GetContent returns a reader for the logs
func (r *LogsResource) GetContent(ctx context.Context, uri string) (io.Reader, string, error) {
	// Parse the URI
	matches := r.uriPattern.FindStringSubmatch(uri)
	if len(matches) < 3 {
		return nil, "", fmt.Errorf("invalid logs URI format. Expected: logs://{environment_id}/{service_name}")
	}

	environmentID := matches[1]
	serviceName := matches[2]
	queryParams := matches[3]

	// Parse query parameters
	tailLines := int64(100)

	if queryParams != "" {
		params := parseQueryParams(queryParams)
		if val, exists := params["tail"]; exists {
			if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
				tailLines = parsed
			}
		}
	}

	// Create logs request
	req := logs.GetLogsRequest{
		EnvironmentID: environmentID,
		ServiceName:   serviceName,
		Follow:        false,
		TailLines:     tailLines,
	}

	// Get logs reader
	reader, err := r.logsService.GetLogsReader(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get logs: %w", err)
	}

	return reader, "text/plain", nil
}

// IsAvailable checks if the resource is available at the given URI
func (r *LogsResource) IsAvailable(ctx context.Context, uri string) bool {
	// Parse the URI
	matches := r.uriPattern.FindStringSubmatch(uri)
	if len(matches) < 3 {
		return false
	}

	environmentID := matches[1]
	serviceName := matches[2]

	// Check if the service exists
	_, err := r.client.FindService(serviceName, environmentID)
	return err == nil
}

// GetResourceTemplate returns the template definition for listing resources
func (r *LogsResource) GetResourceTemplate() ResourceTemplate {
	return ResourceTemplate{
		URITemplate: "logs://{environment_id}/{service_name}",
		Name:        "Service Logs",
		Description: "Get logs from a service in an environment",
		MimeType:    "text/plain",
	}
}

// parseQueryParams parses query string parameters
func parseQueryParams(query string) map[string]string {
	params := make(map[string]string)
	if query == "" {
		return params
	}

	for _, pair := range strings.Split(query, "&") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			params[parts[0]] = parts[1]
		}
	}

	return params
}
