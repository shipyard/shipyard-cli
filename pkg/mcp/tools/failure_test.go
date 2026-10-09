package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/client"
)

const failedBuildResponse = `{
  "type": "application_build",
  "id": "build-1",
  "data": {
    "status": "FAILED",
    "failure": {
      "phase": "build",
      "reason": "BUILDING_IMAGES",
      "reason_text": "Failure building images",
      "retryable": false,
      "services": ["web"],
      "message": null
    },
    "diagnosis": {"summary": "The Dockerfile copies a file that does not exist"},
    "enabled_services": ["db", "web"],
    "services": [
      {
        "name": "web",
        "failing": true,
        "image_build": {"status": "FAILED", "failure_reason": "BUILD_FAILED"},
        "health_check": null,
        "excerpt": {"kind": "build", "lines": ["step failed: [2/2] RUN make", "error: exit code: 2"], "truncated": false}
      },
      {"name": "db", "failing": false, "image_build": null, "health_check": null, "excerpt": null}
    ]
  },
  "links": {}
}`

var errNotFound = errors.New("request failed: 404 not found")

func newFailureTool(rec *recordingRequester, name string) *FailureTool {
	return NewFailureTool(client.Client{Requester: rec, OrgLookupFn: func() string { return "acme" }}, name)
}

func TestFailureTool_GetFailureDetails(t *testing.T) {
	rec := &recordingRequester{resp: []byte(failedBuildResponse)}

	out, err := newFailureTool(rec, "get_failure_details").Execute(
		context.Background(), json.RawMessage(`{"environment_id":"env-123","build_id":"build-1"}`))
	if err != nil {
		t.Fatal(err)
	}

	if rec.method != "GET" || !strings.Contains(rec.uri, "environment/env-123/failure") ||
		!strings.Contains(rec.uri, "build_id=build-1") || !strings.Contains(rec.uri, "org=acme") {
		t.Fatalf("unexpected request %s %s", rec.method, rec.uri)
	}
	for _, want := range []string{
		"Build build-1: FAILED",
		"Failed during: build",
		"Reason: Failure building images (BUILDING_IMAGES)",
		"Failing services: web",
		"Diagnosis: The Dockerfile copies a file that does not exist",
		"Enabled services: db, web (any service not listed is disabled",
		"== web (failing) ==",
		"Image build: FAILED (BUILD_FAILED)",
		"--- build log, last 2 lines ---\nstep failed: [2/2] RUN make\nerror: exit code: 2\n",
		`get_build_logs(environment_id="env-123", build_id="build-1", kind="build", service_name="web")`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// A service with nothing to report gets no section
	if strings.Contains(out, "== db") {
		t.Errorf("db should have no section:\n%s", out)
	}
}

func TestFailureTool_GetFailureDetails_Retryable(t *testing.T) {
	rec := &recordingRequester{resp: []byte(`{
	  "id": "build-1",
	  "data": {
	    "status": "FAILED",
	    "failure": {"phase": null, "reason": null, "reason_text": "The build did not complete. Rebuilding usually resolves this.",
	                "retryable": true, "services": [], "message": null},
	    "diagnosis": null,
	    "enabled_services": ["web"],
	    "services": [{"name": "web", "failing": false, "image_build": null, "health_check": null, "excerpt": null}]
	  }
	}`)}

	out, err := newFailureTool(rec, "get_failure_details").Execute(context.Background(), json.RawMessage(`{"environment_id":"env-123"}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "The build did not complete. Rebuilding usually resolves this.\n") ||
		!strings.Contains(out, `get_environment(environment_id="env-123") first`) ||
		!strings.Contains(out, `rebuild once with rebuild_environment(environment_id="env-123")`) ||
		!strings.Contains(out, "stop and report the failure to the user") {
		t.Errorf("expected rebuild guidance:\n%s", out)
	}
	// Never attribute the failure to Shipyard: the API deliberately withholds that
	if strings.Contains(strings.ToLower(out), "shipyard-side") || strings.Contains(out, "not caused by the app") {
		t.Errorf("retryable output must not blame Shipyard:\n%s", out)
	}
	if strings.Contains(out, "build_id=") || strings.Contains(out, "get_build_logs") {
		t.Errorf("no build_id or log hints expected:\n%s", out)
	}
}

func TestFailureTool_GetFailureDetails_NotFailed(t *testing.T) {
	rec := &recordingRequester{resp: []byte(`{"id":"build-1","data":{"status":"RUNNING","failure":null,"enabled_services":["web"],"services":[]}}`)}

	out, err := newFailureTool(rec, "get_failure_details").Execute(context.Background(), json.RawMessage(`{"environment_id":"env-123"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Build build-1: RUNNING\nThis build did not fail.") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

func TestFailureTool_GetBuildLogs_Request(t *testing.T) {
	rec := &recordingRequester{resp: []byte(`{"id":"build-1","data":{"kind":"crash","services":[]},"links":{}}`)}

	_, err := newFailureTool(rec, "get_build_logs").Execute(context.Background(), json.RawMessage(
		`{"environment_id":"env-123","build_id":"build-1","service_name":"web","kind":"crash","tail":50,"offset":100,"failed_only":false}`))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"environment/env-123/logs", "build_id=build-1", "service=web", "kind=crash", "tail=50", "offset=100", "failed_only=false",
	} {
		if !strings.Contains(rec.uri, want) {
			t.Errorf("request %s missing %q", rec.uri, want)
		}
	}
}

func TestFailureTool_GetBuildLogs_Defaults(t *testing.T) {
	rec := &recordingRequester{resp: []byte(`{"id":"build-1","data":{"kind":"run","services":[]},"links":{}}`)}

	out, err := newFailureTool(rec, "get_build_logs").Execute(context.Background(), json.RawMessage(`{"environment_id":"env-123"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.uri, "kind=run") || strings.Contains(rec.uri, "tail=") || strings.Contains(rec.uri, "failed_only") {
		t.Errorf("unexpected request %s", rec.uri)
	}
	if out != "No run logs stored for build build-1.\n" {
		t.Errorf("unexpected output %q", out)
	}
}

func TestFailureTool_GetBuildLogs_Formatting(t *testing.T) {
	rec := &recordingRequester{resp: []byte(`{
	  "id": "build-1",
	  "data": {"kind": "run", "services": [
	    {"name": "web", "failing": true, "lines": ["b", "c"], "truncated": true, "offset": 3, "total_lines": 10}
	  ]},
	  "links": {"next": "/api/v1/environment/env-123/logs?build_id=build-1&kind=run&service=web&tail=2&offset=5"}
	}`)}

	out, err := newFailureTool(rec, "get_build_logs").Execute(context.Background(), json.RawMessage(`{"environment_id":"env-123","service_name":"web"}`))
	if err != nil {
		t.Fatal(err)
	}

	want := "== web (run log, failing): 2 of 10 lines, ending 3 lines before the end ==\nb\nc\n(earlier lines omitted)\n" +
		"\nOlder lines: get_build_logs(environment_id=\"env-123\", build_id=\"build-1\", kind=\"run\", service_name=\"web\", tail=2, offset=5)\n"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestFailureTool_GetBuildLogs_Validation(t *testing.T) {
	tests := map[string]string{
		"missing environment": `{}`,
		"unknown kind":        `{"environment_id":"env-123","kind":"debug"}`,
		"tail too large":      `{"environment_id":"env-123","tail":5001}`,
		"negative offset":     `{"environment_id":"env-123","offset":-1}`,
		"bad service name":    `{"environment_id":"env-123","service_name":"web; rm -rf /"}`,
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			rec := &recordingRequester{}
			if _, err := newFailureTool(rec, "get_build_logs").Execute(context.Background(), json.RawMessage(params)); err == nil {
				t.Fatal("expected a validation error")
			}
			if rec.uri != "" {
				t.Errorf("no request expected, got %s", rec.uri)
			}
		})
	}
}

func TestFailureTool_APIError(t *testing.T) {
	rec := &recordingRequester{err: errNotFound}

	_, err := newFailureTool(rec, "get_failure_details").Execute(context.Background(), json.RawMessage(`{"environment_id":"env-123"}`))
	if err == nil || !strings.Contains(err.Error(), "env-123") {
		t.Fatalf("expected a not found error naming the environment, got %v", err)
	}
}

func TestExtendedTool_GetBuildHistory_PageSizeLimit(t *testing.T) {
	// The API caps page_size at 100 and returns fewer builds above it without saying so
	rec := &recordingRequester{}
	tool := NewExtendedTool(client.Client{Requester: rec}, "get_build_history")

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"environment_id":"env-123","page_size":101}`)); err == nil {
		t.Fatal("expected a validation error")
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"environment_id":"env-123","page_size":100}`)); err != nil {
		t.Fatal(err)
	}
}
