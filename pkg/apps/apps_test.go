package apps

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactValues(t *testing.T) {
	env := []EnvItem{{Name: "API_KEY", Value: "s3cret-value"}, {Name: "LONGER", Value: "s3cret-value-longer"}, {Name: "FLAG", Value: "on"}}
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
