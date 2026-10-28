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

	"github.com/shipyard/shipyard-cli/pkg/apps"
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
		"status --porcelain --untracked-files=normal --ignore-submodules=none": "",
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
		{name: "origin credentials are not printed", change: map[string]string{"remote get-url origin": "https://user:tok123@gitlab.com/example-org/app.git"}, wantErr: "https://gitlab.com/example-org/app.git"},
		{name: "detached HEAD", change: map[string]string{"rev-parse --abbrev-ref HEAD": "HEAD"}, wantErr: "detached"},
		{name: "branch not on origin", change: map[string]string{"ls-remote --heads origin refs/heads/feature": ""}, wantErr: "not on origin"},
		{
			// The suggested push command must be safe to paste whatever the branch is called
			name:    "branch name is quoted in the push hint",
			change:  map[string]string{"rev-parse --abbrev-ref HEAD": "fix;id", "ls-remote --heads origin refs/heads/fix;id": ""},
			wantErr: "git push -u origin 'fix;id'",
		},
		{name: "local HEAD differs from origin", change: map[string]string{"rev-parse HEAD": "2222222222222222222222222222222222222222"}, wantErr: "differs from origin"},
		{name: "modified tracked file", change: map[string]string{"status --porcelain --untracked-files=normal --ignore-submodules=none": " M main.go"}, wantErr: "uncommitted changes"},
		{name: "untracked file", change: map[string]string{"status --porcelain --untracked-files=normal --ignore-submodules=none": "?? notes.txt"}, wantErr: "uncommitted changes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answers := cleanCheckout()
			for k, v := range tt.change {
				answers[k] = v
			}
			delete(answers, tt.drop)

			co, err := preflight(fakeGit(answers))
			if err != nil && strings.Contains(err.Error(), "tok123") {
				t.Fatalf("err prints the remote's credentials: %v", err)
			}
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
	"compose_file":"docker-compose.yml","services":[{"name":"web","ports":[80],"profiles":[],"enabled_by_default":true},{"name":"api","ports":[8080],"profiles":[],"enabled_by_default":true},{"name":"db","ports":[],"profiles":["db"],"enabled_by_default":false}],
	"routes":[],
	"route_candidates":[{"service":"web","hint":"port 80"},{"service":"api","hint":"port 8080"}],
	"env":[{"name":"API_KEY","kind":"runtime","services":["api"]},{"name":"NPM_TOKEN","kind":"build_arg","services":["web"]}]}`

// planWithRoutes is a plan whose compose file labels its routes, one of them
// named. Core offers route candidates only when the file has no route.
const planWithRoutes = `{"status":"plan","org":"example-org","head_sha":"` + sha + `","compose_file":"docker-compose.yml",
	"services":[{"name":"web","profiles":[],"enabled_by_default":true},{"name":"api","profiles":[],"enabled_by_default":true}],
	"routes":[{"service":"web","route":"/","source":"compose"},{"service":"api","route":"/api","source":"compose"},{"service":"api","route":"/v2","source":"compose"}],
	"route_candidates":[],"env":[]}`

func TestAdd(t *testing.T) {
	tests := []struct {
		name         string
		planStatus   int
		planResp     string
		createStatus int
		createResp   string
		// input answers the prompts in order.
		input   string
		wantErr error
		// wantErrText, when set, must appear in the error instead of matching wantErr.
		wantErrText string
		wantOut     []string
		wantCreate  bool
		checkBody   func(t *testing.T, body map[string]any)
		visitors    []string
	}{
		{
			name:         "created with edited services, route and one env var",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusCreated,
			createResp:   `{"data":{"id":"app-uuid-1","attributes":{"name":"my-app","url":"https://app.example.com"}}}`,
			// name, all services?, which services, main route, API_KEY?, value, NPM_TOKEN?, create?
			input:      "my-app\nn\nweb,api\napi\ny\n" + secretVal + "\nn\ny\n",
			wantOut:    []string{"Org: example-org", `Create app "my-app" in org example-org`, "Services: web, api, db", "API_KEY (runtime)", "NPM_TOKEN (build arg)", "Created app my-app (app-uuid-1)", "URL: https://app.example.com"},
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
				// Core pins a compose file the caller names, so the detected one is not sent
				if _, ok := project["compose_filename"]; ok {
					t.Errorf("compose_filename sent: %v", project["compose_filename"])
				}
				secrets := body["secrets"].([]any)
				if len(secrets) != 1 || secrets[0].(map[string]any)["value"] != secretVal {
					t.Errorf("secrets = %v", secrets)
				}
				// API_KEY is set only on the service the compose file gives it to
				if s, _ := secrets[0].(map[string]any)["services"].([]any); len(s) != 1 || s[0] != "api" {
					t.Errorf("secret services = %v, want [api]", secrets[0].(map[string]any)["services"])
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
		{
			name:         "create needs_action quoting a value inside a message is masked",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusConflict,
			createResp:   `{"status":"needs_action","next_step":{"kind":"fix_env","message":"value ` + secretVal + ` for API_KEY is invalid"}}`,
			input:        "\n\n\ny\n" + secretVal + "\nn\n\n",
			wantErr:      errStopped,
			wantOut:      []string{"Next step: value [REDACTED] for API_KEY is invalid"},
			wantCreate:   true,
		},
		{
			name:         "create error quoting a value is masked",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusBadRequest,
			createResp:   `{"errors":[{"title":"Bad value ` + secretVal + `"}]}`,
			input:        "\n\n\ny\n" + secretVal + "\nn\n\n",
			wantErrText:  "bad value [REDACTED]",
			wantCreate:   true,
		},
		{
			// Someone pushed between preflight and the plan
			name:        "plan for a commit other than local HEAD",
			planStatus:  http.StatusOK,
			planResp:    strings.Replace(planBody, sha, "2222222222222222222222222222222222222222", 1),
			wantErrText: "your checkout is at 1111111. If you just pushed",
		},
		{
			name:       "unknown service is asked again",
			planStatus: http.StatusOK,
			planResp:   planBody,
			// name, all services?, which (bogus, then web), route, API_KEY?, NPM_TOKEN?, create?
			input:   "\nn\nbogus\nweb\nweb\nn\nn\nn\n",
			wantOut: []string{"Not in the compose file: bogus", "Nothing was created."},
		},
		{
			name:       "an answer other than y or n is asked again",
			planStatus: http.StatusOK,
			planResp:   planBody,
			input:      "\nmaybe\n\n\nn\nn\nn\n",
			wantOut:    []string{"Answer y or n", "Nothing was created."},
		},
		{
			name:       "a route that is not a candidate is asked again",
			planStatus: http.StatusOK,
			planResp:   planBody,
			input:      "\n\ndb\nweb\nn\nn\nn\n",
			wantOut:    []string{"Choose one of web, api", "Nothing was created."},
		},
		{
			name:        "input ends before all answers",
			planStatus:  http.StatusOK,
			planResp:    planBody,
			input:       "\n",
			wantErrText: "input ended",
		},
		{
			name:         "a plan without head_sha creates from the local HEAD",
			planStatus:   http.StatusOK,
			planResp:     strings.Replace(planBody, `"head_sha":"`+sha+`",`, "", 1),
			createStatus: http.StatusCreated,
			createResp:   `{"data":{"id":"app-uuid-1","attributes":{"name":"app"}}}`,
			input:        "\n\n\nn\nn\n\n",
			wantOut:      []string{"Created app app (app-uuid-1)"},
			wantCreate:   true,
			checkBody: func(t *testing.T, body map[string]any) {
				if body["head_sha"] != sha {
					t.Errorf("head_sha = %v, want the local HEAD %s", body["head_sha"], sha)
				}
			},
		},
		{
			// A compose file is repo content: an escape sequence in it must not reach the terminal
			name:       "control characters from the plan are not printed",
			planStatus: http.StatusOK,
			planResp: `{"status":"plan","org":"example-org","head_sha":"` + sha + `","compose_file":"docker-compose.yml\u001b[2J",` +
				`"services":["web\u001b]52;c;aGk=\u0007"],"env":[{"name":"KEY\rFAKE","kind":"runtime"}]}`,
			input:   "\n\nn\nn\n",
			wantOut: []string{"Nothing was created."},
		},
		{
			name:         "services behind a compose profile are off by default",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusCreated,
			createResp:   `{"data":{"id":"app-uuid-1","attributes":{"name":"app"}}}`,
			// name, enable web, api?, route, API_KEY?, NPM_TOKEN?, create?
			input:      "\n\n\nn\nn\n\n",
			wantOut:    []string{"db (off by default)", "Enable web, api?"},
			wantCreate: true,
			checkBody: func(t *testing.T, body map[string]any) {
				project := body["projects"].([]any)[0].(map[string]any)
				if got := project["services"].([]any); len(got) != 2 || got[0] != "web" || got[1] != "api" {
					t.Errorf("services = %v, want web and api", got)
				}
			},
		},
		{
			name:         "an empty value is asked for again",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusCreated,
			createResp:   `{"data":{"id":"app-uuid-1","attributes":{"name":"app"}}}`,
			// API_KEY? y, then an empty value, then the value
			input:      "\n\n\ny\n\n" + secretVal + "\nn\n\n",
			wantOut:    []string{"A value is required"},
			wantCreate: true,
			checkBody: func(t *testing.T, body map[string]any) {
				if secrets := body["secrets"].([]any); secrets[0].(map[string]any)["value"] != secretVal {
					t.Errorf("secrets = %v", secrets)
				}
			},
		},
		{
			name:       "plan creating stops before any prompt",
			planStatus: http.StatusOK,
			planResp:   `{"status":"creating","data":{"id":"app-uuid-1"}}`,
			wantErr:    errStopped,
			wantOut:    []string{"still being created by another request"},
		},
		{
			name:       "plan already_exists stops before any prompt",
			planStatus: http.StatusOK,
			planResp:   `{"status":"already_exists","data":{"id":"app-uuid-1"}}`,
			wantErr:    errStopped,
			wantOut:    []string{"already exists"},
		},
		{
			name:       "plan with only a next step stops",
			planStatus: http.StatusOK,
			planResp:   `{"next_step":{"kind":"push_branch","message":"Push the branch"}}`,
			wantErr:    errStopped,
			wantOut:    []string{"Next step: Push the branch"},
		},
		{
			name:         "create 2xx with only a next step stops",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusOK,
			createResp:   `{"next_step":{"kind":"visitors_required","message":"Send visitors"}}`,
			input:        "\n\n\nn\nn\n\n",
			wantErr:      errStopped,
			wantOut:      []string{"Next step: Send visitors"},
			wantCreate:   true,
		},
		{
			// Re-sending a named shipyard.route.<name> label as shipyard.route would
			// duplicate it, and the build fails with DUPLICATE_ROUTES
			name:         "compose routes are not sent back",
			planStatus:   http.StatusOK,
			planResp:     planWithRoutes,
			createStatus: http.StatusCreated,
			createResp:   `{"data":{"id":"app-uuid-1","attributes":{"name":"app"}}}`,
			// name, enable web, api?, create?
			input:      "\n\n\n",
			wantOut:    []string{"/v2 -> api"},
			wantCreate: true,
			checkBody: func(t *testing.T, body map[string]any) {
				project := body["projects"].([]any)[0].(map[string]any)
				if _, ok := project["serviceLabels"]; ok {
					t.Errorf("serviceLabels sent: %v", project["serviceLabels"])
				}
			},
		},
		{
			name:       "an existing app on the branch stops before any prompt",
			planStatus: http.StatusOK,
			planResp:   strings.Replace(planBody, `"routes":[]`, `"existing_app":{"id":"app-uuid-1"},"routes":[]`, 1),
			wantErr:    errStopped,
			wantOut:    []string{"An app already exists for this repo and branch", "app-uuid-1"},
		},
		{
			name:        "a plan with no services stops before any prompt",
			planStatus:  http.StatusOK,
			planResp:    `{"status":"plan","org":"example-org","head_sha":"` + sha + `","services":[],"routes":[],"route_candidates":[],"env":[]}`,
			wantErrText: "no services",
		},
		{
			// Core rejects a profile-only service unless its profile is selected
			name:         "choosing a service behind a profile selects the profile",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusCreated,
			createResp:   `{"data":{"id":"app-uuid-1","attributes":{"name":"app"}}}`,
			// name, enable web, api? n, services, route, API_KEY?, NPM_TOKEN?, create?
			input:      "\nn\nweb,db\n\nn\nn\n\n",
			wantCreate: true,
			checkBody: func(t *testing.T, body map[string]any) {
				project := body["projects"].([]any)[0].(map[string]any)
				if p, _ := project["selectedProfiles"].([]any); len(p) != 1 || p[0] != "db" {
					t.Errorf("selectedProfiles = %v, want [db]", project["selectedProfiles"])
				}
			},
		},
		{
			name:         "an existing app's bypass token is not printed",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusOK,
			createResp:   `{"status":"already_exists","data":{"id":"app-uuid-1","attributes":{"bypass_token":"bt-secret-9"}}}`,
			input:        "\n\n\nn\nn\n\n",
			wantErr:      errStopped,
			wantOut:      []string{"already exists", "app-uuid-1"},
			wantCreate:   true,
		},
		{
			// An org API token has no user to add, so core requires visitors
			name:         "visitors are sent",
			planStatus:   http.StatusOK,
			planResp:     planBody,
			createStatus: http.StatusCreated,
			createResp:   `{"data":{"id":"app-uuid-1","attributes":{"name":"app"}}}`,
			visitors:     []string{"octocat"},
			input:        "\n\n\nn\nn\n\n",
			wantCreate:   true,
			checkBody: func(t *testing.T, body map[string]any) {
				if v, _ := body["visitors"].([]any); len(v) != 1 || v[0] != "octocat" {
					t.Errorf("visitors = %v", body["visitors"])
				}
			},
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
				client:   client.New(requests.New(), func() string { return "" }),
				git:      fakeGit(cleanCheckout()),
				in:       bufio.NewReader(strings.NewReader(tt.input)),
				out:      &out,
				visitors: tt.visitors,
				readSecret: func(in *bufio.Reader) (string, error) {
					line, err := in.ReadString('\n')
					return strings.TrimSpace(line), err
				},
			}
			err := a.run()
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("err = %v, want it to contain %q\noutput:\n%s", err, tt.wantErrText, out.String())
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v\noutput:\n%s", err, tt.wantErr, out.String())
			}
			if err != nil && strings.Contains(err.Error(), secretVal) {
				t.Fatalf("error prints an env value: %v", err)
			}
			if strings.ContainsAny(out.String(), "\x1b\x07\r") {
				t.Fatalf("output holds a control character: %q", out.String())
			}
			if strings.Contains(out.String(), "bt-secret-9") {
				t.Fatalf("output prints a bypass token:\n%s", out.String())
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

func TestOrgName(t *testing.T) {
	tests := []struct {
		name    string
		planned string
		lookup  string
		want    string
	}{
		{"core's planned org wins", "planned-org", "config-org", "planned-org"},
		{"else the org this CLI sends", "", "config-org", "config-org"},
		{"else the account default", "", "", "(your default org)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &adder{client: client.New(requests.New(), func() string { return tt.lookup })}
			if got := a.orgName(apps.PlanResponse{Org: tt.planned}); got != tt.want {
				t.Errorf("orgName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithoutCredentials(t *testing.T) {
	for remote, want := range map[string]string{
		"https://user:tok123@gitlab.com/o/r.git": "https://gitlab.com/o/r.git",
		// url.Parse rejects the port, so the fallback must still drop the token
		"https://user:tok123@gitlab.com:bad/o/r.git": "https://gitlab.com:bad/o/r.git",
		"git@gitlab.com:o/r.git":                     "git@gitlab.com:o/r.git",
	} {
		if got := withoutCredentials(remote); got != want {
			t.Errorf("withoutCredentials(%q) = %q, want %q", remote, got, want)
		}
	}
}

func TestCleanError(t *testing.T) {
	if got := cleanError(errStopped); !errors.Is(got, errStopped) {
		t.Errorf("cleanError lost errStopped: %v", got)
	}
	got := cleanError(errors.New("bad name web\x1b[2J"))
	if strings.ContainsRune(got.Error(), '\x1b') {
		t.Errorf("cleanError = %q, want control characters replaced", got)
	}
	if cleanError(nil) != nil {
		t.Error("cleanError(nil) should be nil")
	}
}

func TestSecretReader(t *testing.T) {
	stdin := strings.NewReader(" padded value \r\nlast")
	in := bufio.NewReader(stdin)
	read := secretReader(stdin)
	for _, want := range []string{" padded value ", "last"} {
		got, err := read(in)
		if err != nil || got != want {
			t.Errorf("read = %q, %v; want %q", got, err, want)
		}
	}
	if _, err := read(in); !errors.Is(err, io.EOF) {
		t.Errorf("read at the end = %v, want io.EOF", err)
	}
}

func TestAddStopsOnPreflight(t *testing.T) {
	answers := cleanCheckout()
	answers["status --porcelain --untracked-files=normal --ignore-submodules=none"] = " M main.go"
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
