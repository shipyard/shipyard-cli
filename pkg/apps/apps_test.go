package apps

import (
	"encoding/json"
	"errors"
	"net/http"
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
	want := []PlanService{{Name: "web", EnabledByDefault: true}, {Name: "debug", EnabledByDefault: false}, {Name: "legacy", EnabledByDefault: true}}
	if len(plan.Services) != len(want) {
		t.Fatalf("services = %+v", plan.Services)
	}
	for i := range want {
		if plan.Services[i] != want[i] {
			t.Errorf("services[%d] = %+v, want %+v", i, plan.Services[i], want[i])
		}
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
	env := []EnvItem{{Name: "API_KEY", Value: "AbCd1234"}, {Name: "NAME", Value: "pässwörd"}, {Name: "HTML", Value: "a<b>c&d"}}
	tests := []struct {
		name string
		body string
	}{
		// The client lower-cases the title's first letter, so masking the
		// message would miss a value at its start
		{"value at the start of a title", `{"errors":[{"title":"AbCd1234 is invalid"}]}`},
		{"non-ASCII value escaped by core", `{"errors":[{"title":"bad value p\u00e4ssw\u00f6rd"}]}`},
		{"HTML characters escaped by core", `{"error":"bad value a\u003cb\u003ec\u0026d"}`},
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
