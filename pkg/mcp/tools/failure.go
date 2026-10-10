package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
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
	// Detail is Shipyard's own message about the customer's config, such as an invalid Compose file
	Detail *string `json:"detail"`
	// LogsAvailable says whether the build deployed services, so they have run and crash logs
	LogsAvailable *bool `json:"logs_available"`
	// NextStep is the API's action for the failure: rebuild_once, check_repository, fix_config, ...
	NextStep string `json:"next_step"`
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
	// LogKind is the log most likely to explain a failing service, if it has one
	LogKind *string `json:"log_kind"`
	// Unavailable means the log store that should hold the service's logs couldn't be read
	Unavailable bool `json:"unavailable"`
}

type failureResponse struct {
	ID   string `json:"id"`
	Data struct {
		Status string `json:"status"`
		// Finished says whether the build has stopped; an unfinished one without a failure is in progress
		Finished  *bool           `json:"finished"`
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
	body, err := t.getWithRetry(uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "failure", apiParams))
	if err != nil {
		return "", logsAPIError("get_failure_details", err, toolParams.EnvironmentID, toolParams.BuildID, "")
	}

	var resp failureResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("unexpected response from the failure details API: %w", err)
	}
	return formatFailureDetails(toolParams.EnvironmentID, &resp), nil
}

// getWithRetry reads the failure or logs API. Its first read of a build's logs can take longer
// than the client waits; the API keeps working and caches the answer for a finished build, so one
// retry after a timeout usually returns it.
func (t *ExtendedTool) getWithRetry(target string) ([]byte, error) {
	body, err := t.client.Requester.Do(http.MethodGet, target, "application/json", nil)
	if err != nil && strings.Contains(err.Error(), "server took too long to respond") {
		body, err = t.client.Requester.Do(http.MethodGet, target, "application/json", nil)
	}
	return body, err
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
		mcpErr.Suggestion = "The service is not enabled in this build. get_failure_details lists the services enabled in a build"
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

// maxRepeatBlock is the longest block of lines checked for back-to-back repeats: a stack trace
// written on every restart of a crash loop is usually well under this.
const maxRepeatBlock = 50

// writeLogLines writes log lines from the customer's build or app, each prefixed so none can
// pass for the tool's own headers or instructions. A block of lines repeated back to back, like
// the same stack trace from every restart of a crash loop, is written once with a count.
func writeLogLines(b *strings.Builder, lines []string) {
	for i := 0; i < len(lines); {
		size, repeats := repeatedBlock(lines[i:])
		for _, line := range lines[i : i+size] {
			b.WriteString("| ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if repeats > 1 {
			block, times := "the line above repeats", "times"
			if size > 1 {
				block = fmt.Sprintf("the %d lines above repeat", size)
			}
			if repeats == 2 {
				times = "time"
			}
			fmt.Fprintf(b, "(%s %d more %s)\n", block, repeats-1, times)
		}
		i += size * repeats
	}
}

// repeatedBlock finds the block at the start of lines that repeats back to back over the most
// lines, counting only repeats that save at least two lines once collapsed into a note. It
// returns a block of one line, once, when nothing repeats.
func repeatedBlock(lines []string) (size, repeats int) {
	size, repeats = 1, 1
	for k := 1; k <= maxRepeatBlock && 2*k <= len(lines); k++ {
		r := 1
		for (r+1)*k <= len(lines) && slices.Equal(lines[:k], lines[r*k:(r+1)*k]) {
			r++
		}
		if (r-1)*k >= 2 && r*k > size*repeats {
			size, repeats = k, r
		}
	}
	return size, repeats
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
	case failure == nil && data.Finished != nil && !*data.Finished:
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
			label := "Health checks"
			if deref(failure.Reason) == "PULLING_IMAGE" {
				label = "Image pulls"
			}
			fmt.Fprintf(&b, "%s: %s\n", label, *failure.Message)
		}
		if failure.Detail != nil {
			fmt.Fprintf(&b, "Detail: %s\n", *failure.Detail)
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

	// A retryable failure isn't described: a failed image would point the agent at the app anyway
	retryable := failure != nil && failure.Retryable
	if summary := imageBuildSummary(data.Services); summary != "" && !retryable {
		fmt.Fprintf(&b, "Image builds: %s\n", summary)
	}

	for _, svc := range data.Services {
		if !hasDetails(svc, retryable) {
			continue
		}
		state := ""
		if svc.Failing {
			state = " (failing)"
		}
		fmt.Fprintf(&b, "\n== %s%s ==\n", svc.Name, state)
		// A healthy service is here for its log; its built image is in the summary
		if svc.ImageBuild != nil && !retryable && (svc.Failing || svc.ImageBuild.Status == "FAILED") {
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
		var hints []string
		for _, svc := range data.Services {
			if svc.Failing && svc.LogKind != nil {
				hints = append(hints, fmt.Sprintf("- get_build_logs(environment_id=%q, build_id=%q, kind=%q, service_name=%q)\n",
					environmentID, resp.ID, *svc.LogKind, svc.Name))
			}
		}
		logsAvailable := failure.LogsAvailable == nil || *failure.LogsAvailable
		if logsAvailable {
			hints = append(hints, fmt.Sprintf("- get_build_logs(environment_id=%q, build_id=%q, kind=\"run\") for every service's run log\n",
				environmentID, resp.ID))
		}
		if len(hints) > 0 {
			b.WriteString("\nMore output:\n" + strings.Join(hints, ""))
		} else {
			fmt.Fprintf(&b, "\nNo service logs: the build stopped during %s, before any service ran.\n", deref(failure.Phase))
		}
		b.WriteString(nextStep(environmentID, failure))
	}
	return b.String()
}

// nextStep turns the API's next_step action into an instruction; an action it doesn't know gets none.
// read_logs needs none: the hints above are the step.
func nextStep(environmentID string, failure *failureSummary) string {
	switch failure.NextStep {
	case "check_repository":
		return fmt.Sprintf("Next step: check that each project's branch still exists and its repository is reachable; "+
			"get_environment(environment_id=%q) lists the projects and branches.\n", environmentID)
	case "check_images":
		return "Next step: the failing services' images could not be pulled, so they never started. Check each one's " +
			"image name and tag in the Compose file, and the registry credentials if the image is private.\n"
	case "fix_config":
		described := ""
		if failure.Detail != nil {
			described = " as the Detail line describes"
		}
		return "Next step: fix the Compose file or Shipyard labels in the project's repository" + described + ", then push the fix.\n"
	case "fix_image_build":
		return "Next step: fix the failing image build; its build log shows the step that failed.\n"
	}
	return ""
}

// hasDetails reports whether a service has more to say than an image status, which
// imageBuildSummary counts instead. A retryable failure's failed images are left out.
func hasDetails(svc failureService, retryable bool) bool {
	return svc.Failing || svc.HealthCheck != nil || svc.Excerpt != nil || svc.Unavailable ||
		(!retryable && svc.ImageBuild != nil && svc.ImageBuild.Status == "FAILED")
}

// imageBuildStatusOrder lists failures first, then the common statuses; others follow by name.
var imageBuildStatusOrder = []string{"FAILED", "BUILT", "STARTED", "QUEUED", "CANCELED"}

// imageBuildSummary counts the services' image builds by status: "1 FAILED, 5 BUILT, 7 CANCELED".
func imageBuildSummary(services []failureService) string {
	counts := map[string]int{}
	for _, svc := range services {
		if svc.ImageBuild != nil {
			counts[svc.ImageBuild.Status]++
		}
	}
	var statuses []string
	for status := range counts {
		statuses = append(statuses, status)
	}
	slices.SortFunc(statuses, func(a, b string) int {
		ai, bi := slices.Index(imageBuildStatusOrder, a), slices.Index(imageBuildStatusOrder, b)
		switch {
		case ai == bi:
			return strings.Compare(a, b)
		case ai == -1:
			return 1
		case bi == -1:
			return -1
		}
		return ai - bi
	})
	parts := make([]string, 0, len(statuses))
	for _, status := range statuses {
		parts = append(parts, fmt.Sprintf("%d %s", counts[status], status))
	}
	return strings.Join(parts, ", ")
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

	body, err := t.getWithRetry(uri.CreateResourceURI("", "environment", toolParams.EnvironmentID, "logs", apiParams))
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
		if len(svc.Lines) == 0 && svc.Offset > 0 && svc.Offset >= svc.TotalLines {
			fmt.Fprintf(&b, "== %s (%s log%s): no lines this far back; the log has only %d lines ==\n",
				svc.Name, resp.Data.Kind, state, svc.TotalLines)
			continue
		}
		fmt.Fprintf(&b, "== %s (%s log%s): %d of %d lines", svc.Name, resp.Data.Kind, state, len(svc.Lines), svc.TotalLines)
		if svc.Offset > 0 {
			fmt.Fprintf(&b, ", ending %d lines before the end", svc.Offset)
		}
		b.WriteString(" ==\n")
		// The omitted lines come before these
		if svc.Truncated {
			b.WriteString("(earlier lines omitted)\n")
		}
		writeLogLines(&b, svc.Lines)
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
