package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests"
)

const secretValue = "s3cret-value-123"

type capturedRequest struct {
	method string
	path   string
	query  string
	body   map[string]any
}

// newAppServer answers every request with status and body, recording the request.
func newAppServer(t *testing.T, status int, body string) (client.Client, *capturedRequest) {
	t.Helper()
	got := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.query = r.Method, r.URL.Path, r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	viper.Set("api_url", server.URL)
	t.Cleanup(func() { viper.Set("api_url", "") })
	t.Setenv("SHIPYARD_API_TOKEN", "fake-token")
	return client.New(requests.New(), func() string { return "example-org" }), got
}

func TestApplicationTools(t *testing.T) {
	needsAction := `{"errors":[{"title":"Install the Shipyard GitHub app"}],"status":"needs_action",` +
		`"next_step":{"kind":"install_github_app","url":"https://example.com/install","message":"Install the Shipyard GitHub app"}}`
	alreadyExists := `{"status":"already_exists","requested_vs_existing":{"services":["api"]},"data":{"id":"app-uuid-1"}}`
	creating := `{"status":"creating","message":"This application is still being created. Retry shortly."}`
	limitReached := `{"status":"needs_action","next_step":{"kind":"plan_limit_reached","message":"Stop one or upgrade"}}`

	createArgs := `{"repo":"example-org/app","branch":"main","app_name":"app","services":["web","api"],` +
		`"routes":{"web":"/","api":"/api"},"head_sha":"abc123",` +
		`"env":[{"name":"API_KEY","value":"` + secretValue + `"},{"name":"NPM_TOKEN","value":"` + secretValue + `x","kind":"build_arg"}]}`

	tests := []struct {
		name       string
		tool       string
		args       string
		status     int
		response   string
		wantMethod string
		wantPath   string
		// wantResult must all appear in the result.
		wantResult []string
		wantErr    string
		checkBody  func(t *testing.T, body map[string]any)
	}{
		{
			name:       "plan",
			tool:       "plan_application",
			args:       `{"repo":"example-org/app","branch":"main"}`,
			status:     http.StatusOK,
			response:   `{"status":"plan","repo":"example-org/app","branch":"main","head_sha":"abc123","services":[{"name":"web","enabled_by_default":true},{"name":"api","enabled_by_default":true}],"env":[{"name":"API_KEY","kind":"runtime"}]}`,
			wantMethod: http.MethodPost,
			wantPath:   "/application/plan",
			wantResult: []string{`"status":"plan"`, `"head_sha":"abc123"`},
			checkBody: func(t *testing.T, body map[string]any) {
				if body["repo"] != "example-org/app" || body["branch"] != "main" {
					t.Errorf("plan body = %v", body)
				}
			},
		},
		{
			name:       "plan needs_action error passes through",
			tool:       "plan_application",
			args:       `{"repo":"example-org/app","branch":"main"}`,
			status:     http.StatusForbidden,
			response:   needsAction,
			wantMethod: http.MethodPost,
			wantPath:   "/application/plan",
			wantResult: []string{needsAction},
		},
		{
			name:    "plan rejects a bare repo name",
			tool:    "plan_application",
			args:    `{"repo":"app","branch":"main"}`,
			wantErr: "owner/name",
		},
		{
			name:       "create",
			tool:       "create_application",
			args:       createArgs,
			status:     http.StatusCreated,
			response:   `{"data":{"type":"application","id":"app-uuid-1","attributes":{"name":"app"}}}`,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantResult: []string{`"status":"created"`, `"id":"app-uuid-1"`, `"name":"API_KEY"`, `"kind":"build_arg"`},
			checkBody: func(t *testing.T, body map[string]any) {
				if body["application_name"] != "app" || body["head_sha"] != "abc123" {
					t.Errorf("create body = %v", body)
				}
				project := body["projects"].([]any)[0].(map[string]any)
				if project["repo_owner"] != "example-org" || project["repo_name"] != "app" || project["branch"] != "main" {
					t.Errorf("project = %v", project)
				}
				labels := project["serviceLabels"].(map[string]any)
				if labels["api"].(map[string]any)["shipyard.route"] != "/api" {
					t.Errorf("serviceLabels = %v", labels)
				}
				secrets := body["secrets"].([]any)
				buildArgs := body["build_args"].([]any)
				if len(secrets) != 1 || secrets[0].(map[string]any)["name"] != "API_KEY" {
					t.Errorf("secrets = %v", secrets)
				}
				if len(buildArgs) != 1 || buildArgs[0].(map[string]any)["name"] != "NPM_TOKEN" {
					t.Errorf("build_args = %v", buildArgs)
				}
			},
		},
		{
			name:       "create needs_action passes through",
			tool:       "create_application",
			args:       createArgs,
			status:     http.StatusBadRequest,
			response:   needsAction,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantResult: []string{needsAction},
		},
		{
			name:       "create already_exists passes through",
			tool:       "create_application",
			args:       createArgs,
			status:     http.StatusOK,
			response:   alreadyExists,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantResult: []string{alreadyExists},
		},
		{
			name:       "create creating passes through",
			tool:       "create_application",
			args:       createArgs,
			status:     http.StatusOK,
			response:   creating,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantResult: []string{creating},
		},
		{
			name:       "create at the environment limit passes the next step through",
			tool:       "create_application",
			args:       createArgs,
			status:     http.StatusConflict,
			response:   limitReached,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantResult: []string{limitReached},
		},
		{
			name:       "create during a GitHub outage is a retryable error",
			tool:       "create_application",
			args:       createArgs,
			status:     http.StatusBadGateway,
			response:   `{"errors":[{"status":502,"title":"Could not read the branch from GitHub. Try again shortly."}]}`,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantErr:    "could not read the branch from GitHub. Try again shortly.",
		},
		{
			name:       "create error that echoes a value is masked",
			tool:       "create_application",
			args:       createArgs,
			status:     http.StatusBadRequest,
			response:   `{"errors":[{"title":"Bad value ` + secretValue + `"}]}`,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantErr:    "bad value [REDACTED]",
		},
		{
			name:       "create needs_action quoting a value inside a message is masked",
			tool:       "create_application",
			args:       createArgs,
			status:     http.StatusConflict,
			response:   `{"status":"needs_action","next_step":{"kind":"fix_env","message":"value ` + secretValue + ` for API_KEY is invalid"}}`,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantResult: []string{`"message":"value [REDACTED] for API_KEY is invalid"`},
		},
		{
			name:    "create rejects an env item without a value",
			tool:    "create_application",
			args:    `{"repo":"example-org/app","branch":"main","head_sha":"abc123","env":[{"name":"API_KEY"}]}`,
			wantErr: "env[0].value is required",
		},
		{
			name:    "create rejects an env item with an empty value",
			tool:    "create_application",
			args:    `{"repo":"example-org/app","branch":"main","head_sha":"abc123","env":[{"name":"API_KEY","value":""}]}`,
			wantErr: "env[0].value is required",
		},
		{
			// Without the plan's commit core would build whatever the branch holds now
			name:    "create requires head_sha",
			tool:    "create_application",
			args:    `{"repo":"example-org/app","branch":"main"}`,
			wantErr: "head_sha is required",
		},
		{
			name:    "create rejects an unknown env kind",
			tool:    "create_application",
			args:    `{"repo":"example-org/app","branch":"main","head_sha":"abc123","services":["web"],"env":[{"name":"A","value":"b","kind":"other"}]}`,
			wantErr: "kind must be",
		},
		{
			name:       "update",
			tool:       "update_application",
			args:       `{"application_id":"app-uuid-1","projects":[{"repo":"app","branch":"feature","routes":{"web":"/new"}}],"remove_repos":["old"]}`,
			status:     http.StatusOK,
			response:   `{"data":{"type":"application","id":"app-uuid-1","attributes":{"name":"app"}}}`,
			wantMethod: http.MethodPatch,
			wantPath:   "/application/app-uuid-1",
			wantResult: []string{`"status":"updated"`, `"id":"app-uuid-1"`},
			checkBody: func(t *testing.T, body map[string]any) {
				project := body["projects"].([]any)[0].(map[string]any)
				if project["repo_name"] != "app" || project["branch"] != "feature" {
					t.Errorf("project = %v", project)
				}
				if _, ok := project["repo_owner"]; ok {
					t.Errorf("bare repo name should not send repo_owner: %v", project)
				}
				if got := body["remove_repos"].([]any); len(got) != 1 || got[0] != "old" {
					t.Errorf("remove_repos = %v", got)
				}
			},
		},
		{
			name:       "update needs_action passes through",
			tool:       "update_application",
			args:       `{"application_id":"app-uuid-1","projects":[{"repo":"app","branch":"gone"}]}`,
			status:     http.StatusBadRequest,
			response:   `{"status":"needs_action","next_step":{"kind":"push_branch","message":"Push branch gone"}}`,
			wantMethod: http.MethodPatch,
			wantPath:   "/application/app-uuid-1",
			wantResult: []string{`"kind":"push_branch"`},
		},
		{
			name:       "update 2xx needs_action passes through",
			tool:       "update_application",
			args:       `{"application_id":"app-uuid-1","projects":[{"repo":"app","branch":"gone"}]}`,
			status:     http.StatusOK,
			response:   `{"status":"needs_action","next_step":{"kind":"push_branch","message":"Push branch gone"}}`,
			wantMethod: http.MethodPatch,
			wantPath:   "/application/app-uuid-1",
			wantResult: []string{`"kind":"push_branch"`},
		},
		{
			name:       "update 2xx already_exists passes through",
			tool:       "update_application",
			args:       `{"application_id":"app-uuid-1","projects":[{"repo":"app","branch":"main"}]}`,
			status:     http.StatusOK,
			response:   alreadyExists,
			wantMethod: http.MethodPatch,
			wantPath:   "/application/app-uuid-1",
			wantResult: []string{alreadyExists},
		},
		{
			name:       "update 2xx creating passes through",
			tool:       "update_application",
			args:       `{"application_id":"app-uuid-1","projects":[{"repo":"app","branch":"main"}]}`,
			status:     http.StatusOK,
			response:   creating,
			wantMethod: http.MethodPatch,
			wantPath:   "/application/app-uuid-1",
			wantResult: []string{creating},
		},
		{
			name:       "update at the environment limit passes the next step through",
			tool:       "update_application",
			args:       `{"application_id":"app-uuid-1","projects":[{"repo":"app","branch":"main"}]}`,
			status:     http.StatusConflict,
			response:   limitReached,
			wantMethod: http.MethodPatch,
			wantPath:   "/application/app-uuid-1",
			wantResult: []string{limitReached},
		},
		{
			// Core's PATCH ignores secrets and build_args, so env would be dropped silently
			name:    "update rejects env",
			tool:    "update_application",
			args:    `{"application_id":"app-uuid-1","env":[{"name":"API_KEY","value":"` + secretValue + `"}]}`,
			wantErr: "put_env_vars",
		},
		{
			name:       "update 2xx with only a next step passes through",
			tool:       "update_application",
			args:       `{"application_id":"app-uuid-1","projects":[{"repo":"app","branch":"main"}]}`,
			status:     http.StatusOK,
			response:   `{"data":{"id":"app-uuid-1"},"next_step":{"kind":"push_branch","message":"Push branch main"}}`,
			wantMethod: http.MethodPatch,
			wantPath:   "/application/app-uuid-1",
			wantResult: []string{`"kind":"push_branch"`},
		},
		{
			name:       "create error quoting a value at the start of its title is masked",
			tool:       "create_application",
			args:       `{"repo":"example-org/app","branch":"main","head_sha":"abc123","services":["web"],"env":[{"name":"API_KEY","value":"S3cret-Value-456"}]}`,
			status:     http.StatusBadRequest,
			response:   `{"errors":[{"title":"S3cret-Value-456 is not a valid key"}]}`,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantErr:    "[REDACTED] is not a valid key",
		},
		{
			name:    "create requires services",
			tool:    "create_application",
			args:    `{"repo":"example-org/app","branch":"main","head_sha":"abc123"}`,
			wantErr: "services is required",
		},
		{
			// Core rejects a create with no name
			name:       "create names the app after the repo by default, and sends visitors",
			tool:       "create_application",
			args:       `{"repo":"example-org/app","branch":"main","head_sha":"abc123","services":["web"],"visitors":["octocat"]}`,
			status:     http.StatusCreated,
			response:   `{"status":"created","data":{"id":"app-uuid-1"}}`,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantResult: []string{`"status":"created"`},
			checkBody: func(t *testing.T, body map[string]any) {
				if body["application_name"] != "app" {
					t.Errorf("application_name = %v, want the repo name", body["application_name"])
				}
				if v, _ := body["visitors"].([]any); len(v) != 1 || v[0] != "octocat" {
					t.Errorf("visitors = %v", body["visitors"])
				}
			},
		},
		{
			name:       "create token inside a next step URL is masked, service names are kept",
			tool:       "create_application",
			args:       `{"repo":"example-org/app","branch":"main","head_sha":"abc123","services":["web","redis"],"env":[{"name":"REDIS_HOST","value":"redis"},{"name":"TOKEN","value":"` + secretValue + `"}]}`,
			status:     http.StatusOK,
			response:   `{"status":"already_exists","requested_vs_existing":{"services":["redis"]},"next_step":{"kind":"fix","url":"https://example.com/fix?token=` + secretValue + `"}}`,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantResult: []string{`"services":["redis"]`, `token=[REDACTED]`},
		},
		{
			name:       "create 2xx with only a next step passes through",
			tool:       "create_application",
			args:       createArgs,
			status:     http.StatusOK,
			response:   `{"next_step":{"kind":"visitors_required","message":"Send visitors"}}`,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantResult: []string{`"kind":"visitors_required"`},
		},
		{
			name:       "plan creating passes through",
			tool:       "plan_application",
			args:       `{"repo":"example-org/app","branch":"main"}`,
			status:     http.StatusOK,
			response:   creating,
			wantMethod: http.MethodPost,
			wantPath:   "/application/plan",
			wantResult: []string{creating},
		},
		{
			name:    "update with nothing to change",
			tool:    "update_application",
			args:    `{"application_id":"app-uuid-1"}`,
			wantErr: "nothing to update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, got := newAppServer(t, tt.status, tt.response)
			out, err := NewApplicationTool(c, tt.tool).Execute(context.Background(), json.RawMessage(tt.args))

			if strings.Contains(out, secretValue) || (err != nil && strings.Contains(err.Error(), secretValue)) {
				t.Fatalf("result leaks an env value: %s %v", out, err)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.wantResult {
				if !strings.Contains(out, want) {
					t.Errorf("result %s does not contain %s", out, want)
				}
			}
			if tt.wantMethod != "" {
				if got.method != tt.wantMethod || got.path != tt.wantPath {
					t.Errorf("request = %s %s, want %s %s", got.method, got.path, tt.wantMethod, tt.wantPath)
				}
				if got.query != "org=example-org" {
					t.Errorf("query = %q, want org=example-org", got.query)
				}
			}
			if tt.checkBody != nil {
				tt.checkBody(t, got.body)
			}
		})
	}
}

// Core's create and update answers carry the app's bypass_token, as
// get_environment's do, so the same rule against printing it applies.
func TestApplicationToolsWarnAboutBypassToken(t *testing.T) {
	for _, name := range []string{"create_application", "update_application"} {
		desc := applicationToolDefinitions[name].Description
		if !strings.Contains(desc, "bypass_token") || !strings.Contains(strings.ToLower(desc), "never print") {
			t.Errorf("%s description must warn never to print the bypass_token, got: %q", name, desc)
		}
	}
}

func TestCreateApplicationProfilesAndDefaultName(t *testing.T) {
	c, got := newAppServer(t, http.StatusCreated, `{"status":"created","data":{"id":"app-uuid-1"}}`)
	args := `{"repo":"example-org/my.site","branch":"main","head_sha":"abc123","services":["web","debug"],"selected_profiles":["debug"]}`
	if _, err := NewApplicationTool(c, "create_application").Execute(context.Background(), json.RawMessage(args)); err != nil {
		t.Fatal(err)
	}
	// Core rejects a name with a dot
	if got.body["application_name"] != "my-site" {
		t.Errorf("application_name = %v, want my-site", got.body["application_name"])
	}
	project := got.body["projects"].([]any)[0].(map[string]any)
	if p, _ := project["selectedProfiles"].([]any); len(p) != 1 || p[0] != "debug" {
		t.Errorf("selectedProfiles = %v", project["selectedProfiles"])
	}
}

func TestUpdateApplicationIDErrorNamesField(t *testing.T) {
	for _, args := range []string{`{"projects":[{"repo":"app"}]}`, `{"application_id":"x"}`} {
		_, err := NewApplicationTool(client.Client{}, "update_application").Execute(context.Background(), json.RawMessage(args))
		if err == nil || !strings.Contains(err.Error(), "application_id") || strings.Contains(err.Error(), "environment_id") {
			t.Errorf("Execute(%s) err = %v, want an application_id error", args, err)
		}
	}
}

// Create and update params carry env values, so nothing they log may hold one.
func TestApplicationToolsDoNotLogValues(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	c, _ := newAppServer(t, http.StatusBadRequest, `{"errors":[{"title":"Bad value `+secretValue+`"}]}`)
	for tool, args := range map[string]string{
		"create_application": `{"repo":"example-org/app","branch":"main","head_sha":"abc123","services":["web"],"env":[{"name":"API_KEY","value":"` + secretValue + `"}]}`,
		// An agent may send create's env to plan by mistake
		"plan_application": `{"repo":"example-org/app","branch":"main","env":[{"name":"API_KEY","value":"` + secretValue + `"}]}`,
	} {
		_, _ = NewApplicationTool(c, tool).Execute(context.Background(), json.RawMessage(args))
	}
	if strings.Contains(logs.String(), secretValue) {
		t.Errorf("logs hold an env value:\n%s", logs.String())
	}
}
