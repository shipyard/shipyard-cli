package apps

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/requests"
)

func TestRedactValues(t *testing.T) {
	env := []EnvItem{
		{Name: "API_KEY", Value: "s3cret-value"}, {Name: "LONGER", Value: "s3cret-value-longer"}, {Name: "FLAG", Value: "on"},
		{Name: "PIN", Value: "48151623"}, {Name: "PEM", Value: "line-one\nline-two"},
	}
	tests := []struct {
		name string
		body string
		want string
	}{
		{"name and value object", `{"secrets":[{"name":"API_KEY","value":"anything"}]}`, `{"secrets":[{"name":"API_KEY","value":"[REDACTED]"}]}`},
		{"sent value anywhere", `{"diff":{"old":"s3cret-value"},"list":["s3cret-value"]}`, `{"diff":{"old":"[REDACTED]"},"list":["[REDACTED]"]}`},
		{"untouched body is byte for byte", `{"status": "plan"}`, `{"status": "plan"}`},
		{"not JSON", `bad s3cret-value-longer here`, `bad [REDACTED] here`},
		{"short values stay in free text", `turn it on`, `turn it on`},
		{"sent value inside a longer string", `{"next_step":{"message":"value s3cret-value for API_KEY is invalid"}}`, `{"next_step":{"message":"value [REDACTED] for API_KEY is invalid"}}`},
		{"sent value inside a string in a list", `{"list":["got s3cret-value-longer"]}`, `{"list":["got [REDACTED]"]}`},
		{"sent value as a JSON number", `{"pin":48151623,"count":2}`, `{"count":2,"pin":"[REDACTED]"}`},
		{"JSON-escaped value in raw text", `bad body {"value":"line-one\nline-two"`, `bad body {"value":"[REDACTED]"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(RedactValues([]byte(tt.body), env))
			if got != tt.want {
				t.Errorf("RedactValues = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRedactStringEscapedValue(t *testing.T) {
	// A raw error body quotes a multi-line value in its JSON-escaped form
	env := []EnvItem{{Name: "PEM", Value: "-----BEGIN KEY-----\nabc\"def\n-----END KEY-----"}}
	got := RedactString(`{"error":"bad key -----BEGIN KEY-----\nabc\"def\n-----END KEY-----"}`, env)
	if strings.Contains(got, "BEGIN KEY") {
		t.Errorf("RedactString = %s, want the escaped value masked", got)
	}
}

func TestRedactValuesClearTextValues(t *testing.T) {
	// REDIS_HOST=redis and BRANCH=main repeat names the request sends in the
	// clear: they are no secret, and masking them would break service names and
	// links. A token is masked everywhere, URLs included.
	env := []EnvItem{{Name: "REDIS_HOST", Value: "redis"}, {Name: "BRANCH", Value: "main"}, {Name: "TOKEN", Value: "secret-token-9"}}
	public := []string{"web", "redis", "main", "example-org/app"}
	body := `{"data":{"id":"main-app"},"next_step":{"message":"push main first","url":"https://example.com/fix?token=secret-token-9"},"services":["web","redis"]}`
	want := `{"data":{"id":"main-app"},"next_step":{"message":"push main first","url":"https://example.com/fix?token=[REDACTED]"},"services":["web","redis"]}`
	if got := string(RedactValues([]byte(body), env, public...)); got != want {
		t.Errorf("RedactValues = %s, want %s", got, want)
	}
}

func TestRedactScalarBody(t *testing.T) {
	// A JSON string is valid JSON with no object to walk
	env := []EnvItem{{Name: "K", Value: "s3cret-value"}}
	if got := string(RedactValues([]byte(`"bad s3cret-value"`), env)); strings.Contains(got, "s3cret-value") {
		t.Errorf("RedactValues = %s", got)
	}
	err := RedactError(requests.NewAPIError(http.StatusBadRequest, []byte(`"bad s3cret-value"`)), env)
	if strings.Contains(err.Error(), "s3cret-value") {
		t.Errorf("RedactError = %s", err)
	}
}

func TestPlanServices(t *testing.T) {
	// Core sends objects; a bare name is read as a service on by default
	var plan PlanResponse
	body := `{"services":[{"name":"web","ports":[80],"profiles":[],"enabled_by_default":true},` +
		`{"name":"debug","ports":[],"profiles":["debug"],"enabled_by_default":false},"legacy"]}`
	if err := json.Unmarshal([]byte(body), &plan); err != nil {
		t.Fatal(err)
	}
	want := []PlanService{
		{Name: "web", EnabledByDefault: true, Profiles: []string{}},
		{Name: "debug", EnabledByDefault: false, Profiles: []string{"debug"}},
		{Name: "legacy", EnabledByDefault: true},
	}
	if !reflect.DeepEqual(plan.Services, want) {
		t.Errorf("services = %+v, want %+v", plan.Services, want)
	}
}

func TestPlanEnvServices(t *testing.T) {
	// Core names the services that use each variable, so a value is set only there
	var plan PlanResponse
	if err := json.Unmarshal([]byte(`{"env":[{"name":"PAY_KEY","kind":"runtime","services":["api"]}]}`), &plan); err != nil {
		t.Fatal(err)
	}
	if got := plan.Env[0].Services; len(got) != 1 || got[0] != "api" {
		t.Errorf("env services = %v", got)
	}
}

func TestCreatePayloadSelectedProfiles(t *testing.T) {
	payload, err := CreatePayload(CreateRequest{
		AppName: "app",
		Project: Project{Repo: "example-org/app", Branch: "main", Services: []string{"web", "debug"}, SelectedProfiles: []string{"debug"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	project := payload["projects"].([]map[string]any)[0]
	if got, _ := project["selectedProfiles"].([]string); len(got) != 1 || got[0] != "debug" {
		t.Errorf("selectedProfiles = %v", project["selectedProfiles"])
	}
}

func TestDefaultAppName(t *testing.T) {
	// Names are unique per org, so the branch keeps a second branch's app from
	// colliding; core allows letters, digits, dashes, underscores and spaces
	tests := []struct{ repo, branch, want string }{
		{"example-org/app", "main", "app-main"},
		{"my_app", "", "my_app"},
	}
	for _, tt := range tests {
		if got := DefaultAppName(tt.repo, tt.branch); got != tt.want {
			t.Errorf("DefaultAppName(%q, %q) = %q, want %q", tt.repo, tt.branch, got, tt.want)
		}
	}
	// Characters core rejects become dashes, plus a 6-hex hash of the original
	got := DefaultAppName("example-org/my.site", "feature/x")
	if !regexp.MustCompile(`^my-site-feature-x-[0-9a-f]{6}$`).MatchString(got) {
		t.Errorf("DefaultAppName(my.site, feature/x) = %q, want my-site-feature-x-<hash>", got)
	}
}

func TestUpdatePayloadRemoveReposAreBareNames(t *testing.T) {
	// Core matches remove_repos by name only and never sees an owner, so
	// "fork/api" would unlink another owner's api: an owner is refused
	if _, err := UpdatePayload(UpdateRequest{RemoveRepos: []string{"example-org/old"}}); err == nil ||
		!strings.Contains(err.Error(), "bare repo name") {
		t.Errorf("err = %v, want an owner/name entry refused", err)
	}
	payload, err := UpdatePayload(UpdateRequest{RemoveRepos: []string{"bare"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := payload["remove_repos"].([]string); !reflect.DeepEqual(got, []string{"bare"}) {
		t.Errorf("remove_repos = %v", got)
	}
}

func TestUpdatePayloadClearsProfiles(t *testing.T) {
	// An explicit empty list turns every profile off; nil leaves them alone
	payload, err := UpdatePayload(UpdateRequest{Projects: []Project{{Repo: "app", SelectedProfiles: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := payload["projects"].([]map[string]any)[0]["selectedProfiles"]; !ok || len(got.([]string)) != 0 {
		t.Errorf("selectedProfiles = %v (sent %v), want []", got, ok)
	}
}

func TestRedactValuesCommonValues(t *testing.T) {
	// Everyday values (a scheme, a port, a host) must not break links and ids;
	// a whole field equal to one is still masked
	env := []EnvItem{{Name: "SCHEME", Value: "https"}, {Name: "PORT", Value: "8080"}, {Name: "HOST", Value: "localhost"}}
	body := `{"data":{"id":"app-8080","url":"https://app.example.com:8080/"},"port":8080,"scheme":"https"}`
	want := `{"data":{"id":"app-8080","url":"https://app.example.com:8080/"},"port":"[REDACTED]","scheme":"[REDACTED]"}`
	if got := string(RedactValues([]byte(body), env)); got != want {
		t.Errorf("RedactValues = %s, want %s", got, want)
	}
}

func TestProjectPayloadNullRouteRemovesLabel(t *testing.T) {
	route := "/api"
	payload, err := UpdatePayload(UpdateRequest{Projects: []Project{{Repo: "app", Routes: map[string]*string{"web": nil, "api": &route}}}})
	if err != nil {
		t.Fatal(err)
	}
	labels := payload["projects"].([]map[string]any)[0]["serviceLabels"].(map[string]any)
	web, ok := labels["web"].(map[string]any)
	if !ok || web[RouteLabel] != nil {
		t.Errorf("web labels = %v, want shipyard.route: null", labels["web"])
	}
	if _, present := web[RouteLabel]; !present {
		t.Errorf("web labels = %v, want shipyard.route sent as null", web)
	}
	if labels["api"].(map[string]any)[RouteLabel] != "/api" {
		t.Errorf("api labels = %v", labels["api"])
	}
}

func TestRedactTrailingData(t *testing.T) {
	// A plain-text body starting with a number decodes as that number alone
	env := []EnvItem{{Name: "K", Value: "supersecretvalue"}}
	for _, body := range []string{"400 bad request: K=supersecretvalue", `{"a":1} supersecretvalue`} {
		if got := string(RedactValues([]byte(body), env)); strings.Contains(got, "supersecretvalue") {
			t.Errorf("RedactValues(%q) = %q", body, got)
		}
		err := RedactError(requests.NewAPIError(http.StatusBadRequest, []byte(body)), env)
		if strings.Contains(err.Error(), "supersecretvalue") {
			t.Errorf("RedactError(%q) = %q", body, err)
		}
	}
}

func TestRedactValuesKeys(t *testing.T) {
	env := []EnvItem{{Name: "K", Value: "s3cret-value"}}
	if got := string(RedactValues([]byte(`{"errors":{"s3cret-value":"invalid"}}`), env)); strings.Contains(got, "s3cret-value") {
		t.Errorf("RedactValues = %s, want the key masked", got)
	}
}

func TestCreatePayloadVisitors(t *testing.T) {
	payload, err := CreatePayload(CreateRequest{
		AppName: "app", Project: Project{Repo: "example-org/app", Branch: "main", Services: []string{"web"}},
		Visitors: []string{"octocat"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := payload["visitors"].([]string); len(got) != 1 || got[0] != "octocat" {
		t.Errorf("visitors = %v", payload["visitors"])
	}
}

func TestRedactValuesDoubleEncoded(t *testing.T) {
	// A field holding serialized JSON carries the value in its escaped form
	env := []EnvItem{{Name: "PEM", Value: "line-one\nline-two"}}
	got := string(RedactValues([]byte(`{"detail":"{\"value\":\"line-one\\nline-two\"}"}`), env))
	if strings.Contains(got, "line-one") {
		t.Errorf("RedactValues = %s, want the escaped value masked", got)
	}
}

func TestRedactError(t *testing.T) {
	env := []EnvItem{{Name: "API_KEY", Value: "AbCd1234"}, {Name: "NAME", Value: "pässwörd"}, {Name: "HTML", Value: "a<b>c&d-99"}}
	tests := []struct {
		name string
		body string
	}{
		// The client lower-cases the title's first letter, so masking the
		// message would miss a value at its start
		{"value at the start of a title", `{"errors":[{"title":"AbCd1234 is invalid"}]}`},
		{"non-ASCII value escaped by core", `{"errors":[{"title":"bad value p\u00e4ssw\u00f6rd"}]}`},
		{"HTML characters escaped by core", `{"error":"bad value a\u003cb\u003ec\u0026d-99"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RedactError(requests.NewAPIError(http.StatusBadRequest, []byte(tt.body)), env)
			for _, v := range []string{"bCd1234", "ssw", "a<b>", `a\u003cb`} {
				if strings.Contains(err.Error(), v) {
					t.Errorf("RedactError = %q, still holds %q", err.Error(), v)
				}
			}
			if _, ok := requests.AsAPIError(err); !ok {
				t.Errorf("RedactError lost the *APIError type: %T", err)
			}
		})
	}
	if got := RedactError(errors.New("timeout AbCd1234"), env).Error(); got != "timeout [REDACTED]" {
		t.Errorf("RedactError(plain) = %q", got)
	}
	if RedactError(nil, env) != nil {
		t.Error("RedactError(nil) should be nil")
	}
}

func TestIsActionBody(t *testing.T) {
	for body, want := range map[string]bool{
		`{"status":"needs_action"}`:                        true,
		`{"status":"already_exists"}`:                      true,
		`{"status":"creating"}`:                            true,
		`{"errors":[],"next_step":{"kind":"push_branch"}}`: true,
		`{"status":"plan"}`:                                false,
		`{"errors":[{"title":"bad"}]}`:                     false,
		`not json`:                                         false,
	} {
		if got := IsActionBody([]byte(body)); got != want {
			t.Errorf("IsActionBody(%s) = %v, want %v", body, got, want)
		}
	}
}

func TestPlanResponseRouteCandidates(t *testing.T) {
	var plan PlanResponse
	if err := json.Unmarshal([]byte(`{"route_candidates":["web",{"service":"api","port":8080}]}`), &plan); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(plan.RouteCandidates))
	for _, c := range plan.RouteCandidates {
		got = append(got, string(c))
	}
	if strings.Join(got, ",") != "web,api" {
		t.Errorf("route candidates = %v", got)
	}
}

func TestCreatePayloadRequiresOwner(t *testing.T) {
	if _, err := CreatePayload(CreateRequest{Project: Project{Repo: "app", Branch: "main"}}); err == nil {
		t.Error("a bare repo name should be rejected on create")
	}
}

func TestRedactValuesNestedUnicodeEscape(t *testing.T) {
	// Serialized JSON inside a field keeps Python's ASCII escapes after the
	// outer decode
	env := []EnvItem{{Name: "PASS", Value: "pässwörd-7V2"}}
	body := `{"detail":"{\"value\":\"pBSu00e4sswBSu00f6rd-7V2\"}"}`
	body = strings.ReplaceAll(body, "BS", "\x5c\x5c")
	if got := string(RedactValues([]byte(body), env)); strings.Contains(got, "rd-7V2") {
		t.Errorf("RedactValues = %s, want the escaped value masked", got)
	}
}

func TestValidAppName(t *testing.T) {
	for name, want := range map[string]bool{"my-app": true, "my app_2": true, "my.app": false, "": false, strings.Repeat("a", 256): false} {
		if got := ValidAppName(name); got != want {
			t.Errorf("ValidAppName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestDefaultAppNameBoundedAndDistinct(t *testing.T) {
	long := DefaultAppName("example-org/"+strings.Repeat("r", 100), strings.Repeat("b", 200))
	if len(long) > 255 || !ValidAppName(long) {
		t.Errorf("DefaultAppName for a long repo and branch = %d chars, valid %v", len(long), ValidAppName(long))
	}
	// Branches that differ only in characters core rejects must not collide
	a, b := DefaultAppName("example-org/web", "feature/x"), DefaultAppName("example-org/web", "feature-x")
	if a == b {
		t.Errorf("feature/x and feature-x both default to %q", a)
	}
	if got := DefaultAppName("example-org/web", "feature-x"); got != "web-feature-x" {
		t.Errorf("a name core accepts as is should stay readable, got %q", got)
	}
}

func TestRedactOverlappingForms(t *testing.T) {
	// A raw value containing another value's escaped form must be masked whole
	inner := "line-one\nline-two"
	outer := `{"pem":"line-one\nline-two","token":"separate-secret-1"}`
	env := []EnvItem{{Name: "PEM", Value: inner}, {Name: "CRED", Value: outer}}
	if got := RedactString("bad credential "+outer, env); strings.Contains(got, "separate-secret-1") {
		t.Errorf("RedactString = %q, leaks the rest of the credential", got)
	}
}
