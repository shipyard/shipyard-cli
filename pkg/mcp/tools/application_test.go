package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	alreadyExists := `{"status":"already_exists","existing_app":{"uuid":"app-uuid-1"},"diff":{"services":{"add":["api"]}},` +
		`"next_step":{"kind":"already_exists","message":"Use update_application"}}`

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
			response:   `{"status":"plan","repo":"example-org/app","branch":"main","head_sha":"abc123","services":["web","api"],"env":[{"name":"API_KEY","kind":"runtime"}]}`,
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
			status:     http.StatusConflict,
			response:   alreadyExists,
			wantMethod: http.MethodPost,
			wantPath:   "/application",
			wantResult: []string{alreadyExists},
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
			name:    "create rejects an unknown env kind",
			tool:    "create_application",
			args:    `{"repo":"example-org/app","branch":"main","env":[{"name":"A","value":"b","kind":"other"}]}`,
			wantErr: "kind must be",
		},
		{
			name:       "update",
			tool:       "update_application",
			args:       `{"application_id":"app-uuid-1","projects":[{"repo":"app","branch":"feature","routes":{"web":"/new"}}],"remove_repos":["old"],"env":[{"name":"API_KEY","value":"` + secretValue + `"}]}`,
			status:     http.StatusOK,
			response:   `{"data":{"type":"application","id":"app-uuid-1","attributes":{"secrets":[{"name":"API_KEY","value":"` + secretValue + `"}]}}}`,
			wantMethod: http.MethodPatch,
			wantPath:   "/application/app-uuid-1",
			wantResult: []string{`"status":"updated"`, `"id":"app-uuid-1"`, `"name":"API_KEY"`},
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
