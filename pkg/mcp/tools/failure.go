package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/schemas"
	"github.com/shipyard/shipyard-cli/pkg/mcp/validation"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

const logKindsHint = "build, run or crash"

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
	// Unavailable means the log store that should hold the service's logs couldn't be read
	Unavailable bool `json:"unavailable"`
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

func (t *ExtendedTool) executeGetFailureDetails(params json.RawMessage) (string, error) {
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
		return "", logsAPIError("get_failure_details", err, toolParams.EnvironmentID, toolParams.BuildID, "")
	}

	var resp failureResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("unexpected response from the failure details API: %w", err)
	}
	return formatFailureDetails(toolParams.EnvironmentID, &resp), nil
}

// logsAPIError names what the API couldn't find: the build or the service, not only the environment.
func logsAPIError(operation string, err error, environmentID, buildID, serviceName string) error {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "build not found"):
		mcpErr := errors.NotFoundError(operation, "build", buildID)
		if buildID == "" {
			mcpErr.Message = fmt.Sprintf("environment '%s' has no builds", environmentID)
		}
		mcpErr.Suggestion = "Use 'get_build_history' to list the environment's builds and their IDs"
		mcpErr.Cause = err
		return mcpErr
	case strings.Contains(msg, "service not found") && serviceName != "":
		mcpErr := errors.NotFoundError(operation, "service", serviceName)
		mcpErr.Suggestion = "The service is not enabled in this build. Use 'get_services' to list the enabled services"
		mcpErr.Cause = err
		return mcpErr
	case strings.Contains(msg, "application not found"):
		mcpErr := errors.NotFoundError(operation, "environment", environmentID)
		mcpErr.Cause = err
		return mcpErr
	}
	return errors.ParseHTTPError(operation, err, environmentID)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// inProgress are the build statuses of a build that is still running: it has not failed yet.
var inProgress = map[string]bool{
	"QUEUED": true, "PREPROCESSING": true, "REPOSITORY_DOWNLOADED": true, "PREPARING": true, "BUILDING": true,
	"DISTRIBUTING": true, "DEPLOYING": true, "STARTING": true, "CONNECTING": true,
}

// writeLogLines writes log lines from the customer's build or app, each prefixed so none can
// pass for the tool's own headers or instructions.
func writeLogLines(b *strings.Builder, lines []string) {
	for _, line := range lines {
		b.WriteString("| ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

// unreadableLogs is said of a service whose log store couldn't be read.
const unreadableLogs = "Logs could not be read right now; try again shortly"

// formatFailureDetails renders the failure response as text for an LLM: headline, services
// and their log excerpts, then next steps, so the tool's guidance always follows the logs.
func formatFailureDetails(environmentID string, resp *failureResponse) string {
	var b strings.Builder
	data := resp.Data
	failure := data.Failure

	fmt.Fprintf(&b, "Build %s: %s\n", resp.ID, data.Status)
	switch {
	case failure == nil && inProgress[data.Status]:
		fmt.Fprintf(&b, "This build is still in progress, so it has not failed yet. Wait for it to finish, or, for an earlier "+
			"failed build, pass its build_id from get_build_history(environment_id=%q).\n", environmentID)
	case failure == nil:
		b.WriteString("This build did not fail.\n")
	case failure.Retryable:
		// The API withholds the details of these failures; pass on its wording as is.
		if text := deref(failure.ReasonText); text != "" {
			fmt.Fprintf(&b, "%s\n", text)
		}
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
	if len(data.EnabledServices) == 0 {
		// The build failed before its services were loaded
		b.WriteString("Enabled services: unknown for this build\n")
	} else {
		fmt.Fprintf(&b, "Enabled services: %s (any service not listed is disabled for this environment)\n",
			strings.Join(data.EnabledServices, ", "))
	}

	for _, svc := range data.Services {
		if svc.ImageBuild == nil && svc.HealthCheck == nil && svc.Excerpt == nil && !svc.Unavailable {
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
			writeLogLines(&b, ex.Lines)
		} else if svc.Unavailable {
			b.WriteString(unreadableLogs + "\n")
		}
	}

	switch {
	case failure == nil:
	case failure.Retryable:
		fmt.Fprintf(&b, "\nNext step: call get_environment(environment_id=%q) first; a new build may already be running, and if `processing` is true, wait for it. "+
			"Otherwise rebuild once with rebuild_environment(environment_id=%q). If that build fails the same way, stop and report the failure to the user instead of changing the app.\n",
			environmentID, environmentID)
	default:
		b.WriteString("\nMore output:\n")
		for _, svc := range data.Services {
			if !svc.Failing {
				continue
			}
			fmt.Fprintf(&b, "- get_build_logs(environment_id=%q, build_id=%q, kind=%q, service_name=%q)\n",
				environmentID, resp.ID, hintKind(svc), svc.Name)
		}
		fmt.Fprintf(&b, "- get_build_logs(environment_id=%q, build_id=%q, kind=\"run\") for every service's run log\n",
			environmentID, resp.ID)
	}
	return b.String()
}

// hintKind is the log most likely to explain a failing service: its excerpt's, else the build
// log of an image that failed to build (the service never ran), else its run log.
func hintKind(svc failureService) string {
	switch {
	case svc.Excerpt != nil:
		return svc.Excerpt.Kind
	case svc.ImageBuild != nil && svc.ImageBuild.Status == "FAILED":
		return "build"
	default:
		return "run"
	}
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
			// Unavailable means the log store that should hold these logs couldn't be read
			Unavailable bool `json:"unavailable"`
		} `json:"services"`
	} `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

func (t *ExtendedTool) executeGetBuildLogs(params json.RawMessage) (string, error) {
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
	if toolParams.Tail < 0 || toolParams.Tail > schemas.MaxBuildLogTail {
		return "", errors.ValidationError("get_build_logs", "tail", fmt.Sprintf("tail must be between 1 and %d; omit it for the default", schemas.MaxBuildLogTail))
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
		return "", logsAPIError("get_build_logs", err, toolParams.EnvironmentID, toolParams.BuildID, toolParams.ServiceName)
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
		if svc.Unavailable {
			fmt.Fprintf(&b, "== %s (%s log%s): %s ==\n", svc.Name, resp.Data.Kind, state, unreadableLogs)
			continue
		}
		fmt.Fprintf(&b, "== %s (%s log%s): %d of %d lines", svc.Name, resp.Data.Kind, state, len(svc.Lines), svc.TotalLines)
		if svc.Offset > 0 {
			fmt.Fprintf(&b, ", ending %d lines before the end", svc.Offset)
		}
		b.WriteString(" ==\n")
		writeLogLines(&b, svc.Lines)
		if svc.Truncated {
			b.WriteString("(earlier lines omitted)\n")
		}
	}
	// The next link pins the build, kind, service, tail and failed_only; the hint keeps them all,
	// so a newer build or a different tail can't shift the page.
	if next := resp.Links.Next; next != "" && len(resp.Data.Services) == 1 {
		svc := resp.Data.Services[0]
		query := linkQuery(next)
		offset, err := strconv.Atoi(query.Get("offset"))
		switch {
		case err != nil:
		case len(svc.Lines) == 0 || offset <= svc.Offset:
			// The stored copy doesn't reach back this far; following the link would return the same page
			b.WriteString("\nOlder lines are no longer stored.\n")
		default:
			failedOnly := ""
			if v := query.Get("failed_only"); v != "" {
				failedOnly = ", failed_only=" + v
			}
			fmt.Fprintf(&b, "\nOlder lines: get_build_logs(environment_id=%q, build_id=%q, kind=%q, service_name=%q%s, tail=%s, offset=%d)\n",
				environmentID, query.Get("build_id"), query.Get("kind"), query.Get("service"), failedOnly, query.Get("tail"), offset)
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
