package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/schemas"
	"github.com/shipyard/shipyard-cli/pkg/mcp/validation"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

// Limits the API enforces too, checked here so a bad call fails with a clear message.
const (
	maxBuildLogTail = 5000
	logKindsHint    = "build, run or crash"
)

var failureToolDefinitions = map[string]ToolDefinition{
	"get_failure_details": {
		Name: "get_failure_details",
		Description: "Explain why an environment's build failed, in one call: the failure phase and reason, " +
			"which services failed, excerpts from their build or crash logs, and which services are enabled " +
			"(any service not listed is disabled, which often explains connection errors to it). " +
			"Works after the environment stopped. Defaults to the latest build; pass build_id for an older one.",
		InputSchema: schemas.FailureDetailsSchema(),
	},
	"get_build_logs": {
		Name: "get_build_logs",
		Description: "Get a build's stored logs: kind=build (image builds), run (service output) or crash " +
			"(crashed containers). Works after the environment stopped or the pods were cleaned up. " +
			"With service_name, returns that service's last `tail` lines, `offset` lines before the end; " +
			"follow the returned offset for older lines. Without service_name, returns the end of every " +
			"service's log, failing services first (kind=build: failed images only unless failed_only=false). " +
			"Hidden env var and secret values are masked.",
		InputSchema: schemas.BuildLogsSchema(),
	},
}

// FailureTool explains failed builds and returns their stored logs.
type FailureTool struct {
	client client.Client
	name   string
}

// NewFailureTool creates a failure-details or build-logs tool.
func NewFailureTool(client client.Client, name string) *FailureTool {
	return &FailureTool{client: client, name: name}
}

// Definition returns the MCP tool definition.
func (t *FailureTool) Definition() ToolDefinition {
	return failureToolDefinitions[t.name]
}

// Execute runs the named tool.
func (t *FailureTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	log.Printf("MCP tool execution started: %s with params: %s", t.name, string(params))
	switch t.name {
	case "get_failure_details":
		return t.executeGetFailureDetails(params)
	case "get_build_logs":
		return t.executeGetBuildLogs(params)
	default:
		return "", fmt.Errorf("unknown operation: %s", t.name)
	}
}

func (t *FailureTool) orgParams() map[string]string {
	params := make(map[string]string)
	if t.client.OrgLookupFn != nil {
		if org := t.client.OrgLookupFn(); org != "" {
			params["org"] = org
		}
	}
	return params
}

type failureSummary struct {
	Phase      *string  `json:"phase"`
	Reason     *string  `json:"reason"`
	ReasonText *string  `json:"reason_text"`
	Retryable  bool     `json:"retryable"`
	Services   []string `json:"services"`
	Message    *string  `json:"message"`
}

type logExcerpt struct {
	Kind      string   `json:"kind"`
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"`
}

type failureService struct {
	Name       string `json:"name"`
	Failing    bool   `json:"failing"`
	ImageBuild *struct {
		Status        string  `json:"status"`
		FailureReason *string `json:"failure_reason"`
	} `json:"image_build"`
	HealthCheck *string     `json:"health_check"`
	Excerpt     *logExcerpt `json:"excerpt"`
}

type failureResponse struct {
	ID   string `json:"id"`
	Data struct {
		Status    string          `json:"status"`
		Failure   *failureSummary `json:"failure"`
		Diagnosis *struct {
			Summary string `json:"summary"`
		} `json:"diagnosis"`
		EnabledServices []string         `json:"enabled_services"`
		Services        []failureService `json:"services"`
	} `json:"data"`
}

func (t *FailureTool) executeGetFailureDetails(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		BuildID       string `json:"build_id,omitempty"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("get_failure_details", "parameters", err.Error())
	}
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("get_failure_details", "environment_id", err.Error())
	}

	apiParams := t.orgParams()
	if toolParams.BuildID != "" {
		apiParams["build_id"] = toolParams.BuildID
	}
	body, err := t.client.Requester.Do(
		http.MethodGet,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "failure", apiParams),
		"application/json",
		nil,
	)
	if err != nil {
		return "", errors.ParseHTTPError("get_failure_details", err, toolParams.EnvironmentID)
	}

	var resp failureResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("unexpected response from the failure details API: %w", err)
	}
	return formatFailureDetails(toolParams.EnvironmentID, &resp), nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// formatFailureDetails renders the failure response as text for an LLM: headline, services, excerpts, next steps.
func formatFailureDetails(environmentID string, resp *failureResponse) string {
	var b strings.Builder
	data := resp.Data
	failure := data.Failure

	fmt.Fprintf(&b, "Build %s: %s\n", resp.ID, data.Status)
	switch {
	case failure == nil:
		b.WriteString("This build did not fail.\n")
	case failure.Retryable:
		// The API withholds the details of these failures; pass on its wording as is.
		if text := deref(failure.ReasonText); text != "" {
			fmt.Fprintf(&b, "%s\n", text)
		}
		fmt.Fprintf(&b, "Next step: call get_environment(environment_id=%q) first; a new build may already be running, and if `processing` is true, wait for it. "+
			"Otherwise rebuild once with rebuild_environment(environment_id=%q). If that build fails the same way, stop and report the failure to the user instead of changing the app.\n",
			environmentID, environmentID)
	default:
		if phase := deref(failure.Phase); phase != "" {
			fmt.Fprintf(&b, "Failed during: %s\n", phase)
		}
		if text, code := deref(failure.ReasonText), deref(failure.Reason); text != "" && code != "" {
			fmt.Fprintf(&b, "Reason: %s (%s)\n", text, code)
		} else if text+code != "" {
			fmt.Fprintf(&b, "Reason: %s\n", text+code)
		}
		if len(failure.Services) > 0 {
			fmt.Fprintf(&b, "Failing services: %s\n", strings.Join(failure.Services, ", "))
		}
		if failure.Message != nil {
			fmt.Fprintf(&b, "Health checks: %s\n", *failure.Message)
		}
	}
	if data.Diagnosis != nil && data.Diagnosis.Summary != "" {
		fmt.Fprintf(&b, "Diagnosis: %s\n", data.Diagnosis.Summary)
	}
	fmt.Fprintf(&b, "Enabled services: %s (any service not listed is disabled for this environment)\n",
		strings.Join(data.EnabledServices, ", "))

	for _, svc := range data.Services {
		if svc.ImageBuild == nil && svc.HealthCheck == nil && svc.Excerpt == nil {
			continue
		}
		state := ""
		if svc.Failing {
			state = " (failing)"
		}
		fmt.Fprintf(&b, "\n== %s%s ==\n", svc.Name, state)
		if svc.ImageBuild != nil {
			reason := ""
			if r := deref(svc.ImageBuild.FailureReason); r != "" {
				reason = " (" + r + ")"
			}
			fmt.Fprintf(&b, "Image build: %s%s\n", svc.ImageBuild.Status, reason)
		}
		if svc.HealthCheck != nil {
			fmt.Fprintf(&b, "Health check: %s\n", *svc.HealthCheck)
		}
		if ex := svc.Excerpt; ex != nil {
			truncated := ""
			if ex.Truncated {
				truncated = ", earlier lines omitted"
			}
			fmt.Fprintf(&b, "--- %s log, last %d lines%s ---\n", ex.Kind, len(ex.Lines), truncated)
			for _, line := range ex.Lines {
				b.WriteString(line)
				b.WriteByte('\n')
			}
		}
	}

	if failure != nil && !failure.Retryable {
		b.WriteString("\nMore output:\n")
		for _, svc := range data.Services {
			if !svc.Failing {
				continue
			}
			kind := "run"
			if svc.Excerpt != nil {
				kind = svc.Excerpt.Kind
			}
			fmt.Fprintf(&b, "- get_build_logs(environment_id=%q, build_id=%q, kind=%q, service_name=%q)\n",
				environmentID, resp.ID, kind, svc.Name)
		}
		fmt.Fprintf(&b, "- get_build_logs(environment_id=%q, build_id=%q, kind=\"run\") for every service's run log\n",
			environmentID, resp.ID)
	}
	return b.String()
}

type buildLogsResponse struct {
	ID   string `json:"id"`
	Data struct {
		Kind     string `json:"kind"`
		Services []struct {
			Name       string   `json:"name"`
			Failing    bool     `json:"failing"`
			Lines      []string `json:"lines"`
			Truncated  bool     `json:"truncated"`
			Offset     int      `json:"offset"`
			TotalLines int      `json:"total_lines"`
		} `json:"services"`
	} `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

func (t *FailureTool) executeGetBuildLogs(params json.RawMessage) (string, error) {
	var toolParams struct {
		EnvironmentID string `json:"environment_id"`
		BuildID       string `json:"build_id,omitempty"`
		ServiceName   string `json:"service_name,omitempty"`
		Kind          string `json:"kind,omitempty"`
		Tail          int    `json:"tail,omitempty"`
		Offset        int    `json:"offset,omitempty"`
		FailedOnly    *bool  `json:"failed_only,omitempty"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("get_build_logs", "parameters", err.Error())
	}
	if err := validation.ValidateEnvironmentID(toolParams.EnvironmentID); err != nil {
		return "", errors.ValidationError("get_build_logs", "environment_id", err.Error())
	}
	if toolParams.ServiceName != "" {
		if err := validation.ValidateServiceName(toolParams.ServiceName); err != nil {
			return "", errors.ValidationError("get_build_logs", "service_name", err.Error())
		}
	}
	if toolParams.Kind == "" {
		toolParams.Kind = "run"
	}
	if toolParams.Kind != "build" && toolParams.Kind != "run" && toolParams.Kind != "crash" {
		return "", errors.ValidationError("get_build_logs", "kind", "kind must be "+logKindsHint)
	}
	if toolParams.Tail < 0 || toolParams.Tail > maxBuildLogTail {
		return "", errors.ValidationError("get_build_logs", "tail", fmt.Sprintf("tail must be between 1 and %d; omit it for the default", maxBuildLogTail))
	}
	if toolParams.Offset < 0 {
		return "", errors.ValidationError("get_build_logs", "offset", "offset must be non-negative")
	}

	apiParams := t.orgParams()
	apiParams["kind"] = toolParams.Kind
	if toolParams.BuildID != "" {
		apiParams["build_id"] = toolParams.BuildID
	}
	if toolParams.ServiceName != "" {
		apiParams["service"] = toolParams.ServiceName
	}
	if toolParams.Tail > 0 {
		apiParams["tail"] = strconv.Itoa(toolParams.Tail)
	}
	if toolParams.Offset > 0 {
		apiParams["offset"] = strconv.Itoa(toolParams.Offset)
	}
	if toolParams.FailedOnly != nil {
		apiParams["failed_only"] = strconv.FormatBool(*toolParams.FailedOnly)
	}

	body, err := t.client.Requester.Do(
		http.MethodGet,
		uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "logs", apiParams),
		"application/json",
		nil,
	)
	if err != nil {
		return "", errors.ParseHTTPError("get_build_logs", err, toolParams.EnvironmentID)
	}

	var resp buildLogsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("unexpected response from the logs API: %w", err)
	}
	return formatBuildLogs(toolParams.EnvironmentID, &resp), nil
}

// formatBuildLogs renders a logs response as text, one section per service, with how to page further.
func formatBuildLogs(environmentID string, resp *buildLogsResponse) string {
	var b strings.Builder
	if len(resp.Data.Services) == 0 {
		return fmt.Sprintf("No %s logs stored for build %s.\n", resp.Data.Kind, resp.ID)
	}
	for _, svc := range resp.Data.Services {
		state := ""
		if svc.Failing {
			state = ", failing"
		}
		fmt.Fprintf(&b, "== %s (%s log%s): %d of %d lines", svc.Name, resp.Data.Kind, state, len(svc.Lines), svc.TotalLines)
		if svc.Offset > 0 {
			fmt.Fprintf(&b, ", ending %d lines before the end", svc.Offset)
		}
		b.WriteString(" ==\n")
		for _, line := range svc.Lines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if svc.Truncated {
			b.WriteString("(earlier lines omitted)\n")
		}
	}
	// The next link pins the build, kind, service and tail; the hint keeps them all, so a newer
	// build or a different tail can't shift the page.
	if next := resp.Links.Next; next != "" {
		if query := linkQuery(next); query.Get("offset") != "" {
			fmt.Fprintf(&b, "\nOlder lines: get_build_logs(environment_id=%q, build_id=%q, kind=%q, service_name=%q, tail=%s, offset=%s)\n",
				environmentID, query.Get("build_id"), query.Get("kind"), query.Get("service"), query.Get("tail"), query.Get("offset"))
		}
	}
	return b.String()
}

// linkQuery returns the query parameters of a link the API returned.
func linkQuery(link string) url.Values {
	parsed, err := url.Parse(link)
	if err != nil {
		return url.Values{}
	}
	return parsed.Query()
}
