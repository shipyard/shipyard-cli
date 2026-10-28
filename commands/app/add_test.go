package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests"
)

const (
	sha       = "1111111111111111111111111111111111111111"
	secretVal = "s3cret-value-123"
)

// fakeGit answers git commands from a table keyed by the joined args. A
// missing key fails the command.
func fakeGit(answers map[string]string) gitRunner {
	return func(args ...string) (string, error) {
		out, ok := answers[strings.Join(args, " ")]
		if !ok {
			return "", errors.New("fatal: not configured")
		}
		return out, nil
	}
}

func cleanCheckout() map[string]string {
	return map[string]string{
		"remote get-url origin":                       "git@github.com:example-org/app.git",
		"rev-parse --abbrev-ref HEAD":                 "feature",
		"ls-remote --heads origin refs/heads/feature": sha + "\trefs/heads/feature",
		"rev-parse HEAD":                              sha,
		"status --porcelain":                          "",
	}
}

func TestPreflight(t *testing.T) {
	tests := []struct {
		name    string
		change  map[string]string
		drop    string
		wantErr string
	}{
		{name: "clean pushed branch"},
		{name: "no origin", drop: "remote get-url origin", wantErr: "no git remote named origin"},
		{name: "origin not on GitHub", change: map[string]string{"remote get-url origin": "https://gitlab.com/example-org/app.git"}, wantErr: "not a GitHub repo"},
		{name: "detached HEAD", change: map[string]string{"rev-parse --abbrev-ref HEAD": "HEAD"}, wantErr: "detached"},
		{name: "branch not on origin", change: map[string]string{"ls-remote --heads origin refs/heads/feature": ""}, wantErr: "not on origin"},
		{name: "local HEAD differs from origin", change: map[string]string{"rev-parse HEAD": "2222222222222222222222222222222222222222"}, wantErr: "differs from origin"},
		{name: "modified tracked file", change: map[string]string{"status --porcelain": " M main.go"}, wantErr: "uncommitted changes"},
		{name: "untracked file", change: map[string]string{"status --porcelain": "?? notes.txt"}, wantErr: "uncommitted changes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answers := cleanCheckout()
			for k, v := range tt.change {
				answers[k] = v
			}
			delete(answers, tt.drop)

			co, err := preflight(fakeGit(answers))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := checkout{Repo: "example-org/app", Branch: "feature", HeadSHA: sha}
			if co != want {
				t.Errorf("checkout = %+v, want %+v", co, want)
			}
		})
	}
}

func TestParseGitHubRemote(t *testing.T) {
	tests := []struct {
		remote string
		want   string
		ok     bool
	}{
		{"https://github.com/example-org/app.git", "example-org/app", true},
		{"https://github.com/example-org/app", "example-org/app", true},
		{"https://user@github.com/example-org/app.git", "example-org/app", true},
		{"git@github.com:example-org/app.git", "example-org/app", true},
		{"ssh://git@github.com/example-org/app.git", "example-org/app", true},
		{"https://gitlab.com/example-org/app.git", "", false},
		{"https://github.com.evil.example/example-org/app.git", "", false},
		{"/srv/git/app.git", "", false},
	}
	for _, tt := range tests {
		got, ok := parseGitHubRemote(tt.remote)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseGitHubRemote(%q) = %q, %v; want %q, %v", tt.remote, got, ok, tt.want, tt.ok)
		}
	}
}

type apiCall struct {
	method, path string
	body         map[string]any
}

const planBody = `{"status":"plan","org":"example-org","repo":"example-org/app","branch":"feature","head_sha":"` + sha + `",
	"compose_file":"docker-compose.yml","services":["web","api","db"],
	"routes":[{"service":"web","route":"/"},{"service":"api","route":"/api"}],
	"route_candidates":["web","api"],
	"env":[{"name":"API_KEY","kind":"runtime"},{"name":"NPM_TOKEN","kind":"build_arg"}]}`

func TestAdd(t *testing.T) {
	tests := []struct {
		name         string
		planStatus   int
		planResp     string
		createStatus int
		createResp   string
		// input answers the prompts in order.
		input      string
		wantErr    error
		wantOut    []string
		wantCreate bool
		checkBody  func(t *testing.T, body map[string]any)
	}{
		{
			name:         "created with edited services, route and one env var",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusCreated,
			createResp:   `{"data":{"id":"app-uuid-1","attributes":{"name":"my-app","url":"https://app.example.com"}}}`,
			// name, all services?, which services, main route, API_KEY?, value, NPM_TOKEN?, create?
			input:      "my-app\nn\nweb,api\napi\ny\n" + secretVal + "\nn\ny\n",
			wantOut:    []string{"Services: web, api, db", "/api -> api", "API_KEY (runtime)", "NPM_TOKEN (build arg)", "Created app my-app (app-uuid-1)", "URL: https://app.example.com"},
			wantCreate: true,
			checkBody: func(t *testing.T, body map[string]any) {
				if body["application_name"] != "my-app" || body["head_sha"] != sha {
					t.Errorf("body = %v", body)
				}
				project := body["projects"].([]any)[0].(map[string]any)
				if got := project["services"].([]any); len(got) != 2 {
					t.Errorf("services = %v", got)
				}
				labels := project["serviceLabels"].(map[string]any)
				if _, ok := labels["web"]; ok || labels["api"].(map[string]any)["shipyard.route"] != "/" {
					t.Errorf("serviceLabels = %v, want only api on /", labels)
				}
				secrets := body["secrets"].([]any)
				if len(secrets) != 1 || secrets[0].(map[string]any)["value"] != secretVal {
					t.Errorf("secrets = %v", secrets)
				}
				if _, ok := body["build_args"]; ok {
					t.Errorf("NPM_TOKEN was declined but sent: %v", body["build_args"])
				}
			},
		},
		{
			name:       "plan needs action",
			planStatus: http.StatusOK,
			planResp:   `{"status":"needs_action","next_step":{"kind":"install_github_app","url":"https://example.com/install","message":"Install the Shipyard GitHub app on example-org"}}`,
			wantErr:    errStopped,
			wantOut:    []string{"Next step: Install the Shipyard GitHub app", "Open: https://example.com/install"},
		},
		{
			name:       "plan error with next step",
			planStatus: http.StatusPaymentRequired,
			planResp:   `{"errors":[{"title":"Plan limit reached"}],"status":"needs_action","next_step":{"kind":"plan_limit_reached","message":"Upgrade your plan"}}`,
			wantErr:    errStopped,
			wantOut:    []string{"Next step: Upgrade your plan"},
		},
		{
			// Core answers 200 with the existing app and what the request asked for that it lacks
			name:         "already exists shows what differs",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusOK,
			createResp:   `{"status":"already_exists","requested_vs_existing":{"services":["db"]},"data":{"id":"app-uuid-1"}}`,
			input:        "\n\n\ny\n" + secretVal + "\nn\n\n",
			wantErr:      errStopped,
			wantOut:      []string{"already exists", "requested_vs_existing:", `"db"`, "data:", `"app-uuid-1"`},
			wantCreate:   true,
		},
		{
			name:         "another request is still creating the app",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusOK,
			createResp:   `{"status":"creating","message":"This application is still being created. Retry shortly.","data":{"id":"app-uuid-1"}}`,
			input:        "\n\n\ny\n" + secretVal + "\nn\n\n",
			wantErr:      errStopped,
			wantOut:      []string{"still being created by another request"},
			wantCreate:   true,
		},
		{
			name:         "create at the environment limit",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusConflict,
			createResp:   `{"status":"needs_action","next_step":{"kind":"plan_limit_reached","message":"This organization is at its limit of 2 running environments."}}`,
			input:        "\n\n\ny\n" + secretVal + "\nn\n\n",
			wantErr:      errStopped,
			wantOut:      []string{"Next step: This organization is at its limit of 2 running environments."},
			wantCreate:   true,
		},
		{
			name:       "declined",
			planStatus: http.StatusOK,
			planResp:   planBody,
			input:      "\n\n\nn\nn\nn\n",
			wantOut:    []string{"Nothing was created."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []apiCall
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				call := apiCall{method: r.Method, path: r.URL.Path}
				_ = json.Unmarshal(raw, &call.body)
				calls = append(calls, call)
				switch r.URL.Path {
				case "/application/plan":
					w.WriteHeader(tt.planStatus)
					_, _ = w.Write([]byte(tt.planResp))
				case "/application":
					w.WriteHeader(tt.createStatus)
					_, _ = w.Write([]byte(tt.createResp))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			viper.Set("api_url", server.URL)
			defer viper.Set("api_url", "")
			t.Setenv("SHIPYARD_API_TOKEN", "fake-token")

			var out bytes.Buffer
			a := &adder{
				client: client.New(requests.New(), func() string { return "" }),
				git:    fakeGit(cleanCheckout()),
				in:     bufio.NewReader(strings.NewReader(tt.input)),
				out:    &out,
				readSecret: func(in *bufio.Reader) (string, error) {
					line, err := in.ReadString('\n')
					return strings.TrimSpace(line), err
				},
			}
			err := a.run()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v\noutput:\n%s", err, tt.wantErr, out.String())
			}
			if strings.Contains(out.String(), secretVal) {
				t.Fatalf("output prints an env value:\n%s", out.String())
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output does not contain %q:\n%s", want, out.String())
				}
			}

			if len(calls) == 0 || calls[0].path != "/application/plan" || calls[0].body["repo"] != "example-org/app" || calls[0].body["branch"] != "feature" {
				t.Fatalf("first call = %+v, want the plan for example-org/app@feature", calls)
			}
			created := len(calls) > 1 && calls[1].method == http.MethodPost && calls[1].path == "/application"
			if created != tt.wantCreate {
				t.Fatalf("create called = %v, want %v", created, tt.wantCreate)
			}
			if tt.checkBody != nil {
				tt.checkBody(t, calls[1].body)
			}
		})
	}
}

func TestAddStopsOnPreflight(t *testing.T) {
	answers := cleanCheckout()
	answers["status --porcelain"] = " M main.go"
	a := &adder{
		client: client.New(requests.New(), func() string { return "" }),
		git:    fakeGit(answers),
		in:     bufio.NewReader(strings.NewReader("")),
		out:    io.Discard,
	}
	if err := a.run(); err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("err = %v, want the uncommitted changes refusal", err)
	}
}
