// Package apps calls core's application create, reconfigure and plan APIs. The
// MCP tools and `shipyard add` share it, so both send the same bodies.
package apps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

const (
	applicationResource = "application"
	// planResource is core's read-only plan endpoint: POST /api/v1/application/plan.
	planResource = "application/plan"

	// RouteLabel is the compose label that sets a service's route.
	RouteLabel = "shipyard.route"
)

// Env var kinds: runtime vars are sent as secrets, build args as build_args.
const (
	KindRuntime  = "runtime"
	KindBuildArg = "build_arg"
)

// Response statuses core sets when the agent or user must act before a create
// can go ahead.
const (
	StatusPlan          = "plan"
	StatusNeedsAction   = "needs_action"
	StatusAlreadyExists = "already_exists"
)

// NextStep is what core says to do about a needs_action or already_exists answer.
type NextStep struct {
	Kind    string `json:"kind"`
	URL     string `json:"url,omitempty"`
	Message string `json:"message"`
}

// EnvName is one variable the plan found, without a value.
type EnvName struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// PlanResponse is the body of POST /application/plan.
type PlanResponse struct {
	Status          string          `json:"status"`
	Org             string          `json:"org"`
	Repo            string          `json:"repo"`
	Branch          string          `json:"branch"`
	HeadSHA         string          `json:"head_sha"`
	ComposeFile     string          `json:"compose_file"`
	Services        []string        `json:"services"`
	Routes          json.RawMessage `json:"routes,omitempty"`
	RouteCandidates []string        `json:"route_candidates"`
	Env             []EnvName       `json:"env"`
	ExistingApp     json.RawMessage `json:"existing_app,omitempty"`
	NextStep        *NextStep       `json:"next_step,omitempty"`
}

// ActionResponse is the part of a create or update answer that says whether
// the user must act first.
type ActionResponse struct {
	Status   string    `json:"status"`
	NextStep *NextStep `json:"next_step,omitempty"`
}

// EnvItem is a variable to set on the app. Value is a secret: never log it or
// put it in a result.
type EnvItem struct {
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	Kind     string   `json:"kind,omitempty"`
	Hidden   *bool    `json:"hidden,omitempty"`
	Services []string `json:"services,omitempty"`
}

// Project is one repo of an app.
type Project struct {
	// Repo is owner/name, or a bare name for a repo already linked to the app.
	Repo            string
	Branch          string
	Services        []string
	ComposeFilename string
	// Routes maps a service to its shipyard.route label value.
	Routes map[string]string
}

// CreateRequest is the input of Create.
type CreateRequest struct {
	AppName string
	Project Project
	Env     []EnvItem
	// HeadSHA is the commit the plan saw, so core can refuse a stale plan.
	HeadSHA string
}

// UpdateRequest is the input of Update. Only the fields set are changed.
type UpdateRequest struct {
	Projects    []Project
	RemoveRepos []string
	Env         []EnvItem
}

// SplitRepo splits "owner/name". A bare name returns an empty owner.
func SplitRepo(repo string) (owner, name string, err error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "", "", fmt.Errorf("repo is required")
	}
	parts := strings.Split(repo, "/")
	switch {
	case len(parts) == 1:
		return "", parts[0], nil
	case len(parts) == 2 && parts[0] != "" && parts[1] != "":
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("repo must be owner/name, got %q", repo)
	}
}

func projectPayload(p Project, requireOwner bool) (map[string]any, error) {
	owner, name, err := SplitRepo(p.Repo)
	if err != nil {
		return nil, err
	}
	if requireOwner && owner == "" {
		return nil, fmt.Errorf("repo must be owner/name, got %q", p.Repo)
	}
	out := map[string]any{"repo_name": name}
	if owner != "" {
		out["repo_owner"] = owner
	}
	if p.Branch != "" {
		out["branch"] = p.Branch
	}
	if p.Services != nil {
		out["services"] = p.Services
	}
	if p.ComposeFilename != "" {
		out["compose_filename"] = p.ComposeFilename
	}
	if len(p.Routes) > 0 {
		labels := make(map[string]any, len(p.Routes))
		for svc, route := range p.Routes {
			labels[svc] = map[string]string{RouteLabel: route}
		}
		out["serviceLabels"] = labels
	}
	return out, nil
}

// envPayload splits env items into core's secrets and build_args lists.
func envPayload(items []EnvItem) (secrets, buildArgs []map[string]any, err error) {
	for i, v := range items {
		if v.Name == "" {
			return nil, nil, fmt.Errorf("env[%d].name is required", i)
		}
		entry := map[string]any{"name": v.Name, "value": v.Value}
		if v.Hidden != nil {
			entry["hidden"] = *v.Hidden
		}
		if v.Services != nil {
			entry["services"] = v.Services
		}
		switch v.Kind {
		case "", KindRuntime:
			secrets = append(secrets, entry)
		case KindBuildArg:
			buildArgs = append(buildArgs, entry)
		default:
			return nil, nil, fmt.Errorf("env[%d].kind must be %s or %s, got %q", i, KindRuntime, KindBuildArg, v.Kind)
		}
	}
	return secrets, buildArgs, nil
}

func addEnv(payload map[string]any, items []EnvItem) error {
	secrets, buildArgs, err := envPayload(items)
	if err != nil {
		return err
	}
	if secrets != nil {
		payload["secrets"] = secrets
	}
	if buildArgs != nil {
		payload["build_args"] = buildArgs
	}
	return nil
}

// CreatePayload builds the POST /application body.
func CreatePayload(r CreateRequest) (map[string]any, error) {
	if r.Project.Branch == "" {
		return nil, fmt.Errorf("branch is required")
	}
	project, err := projectPayload(r.Project, true)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"projects": []map[string]any{project}}
	if r.AppName != "" {
		payload["application_name"] = r.AppName
	}
	if r.HeadSHA != "" {
		payload["head_sha"] = r.HeadSHA
	}
	if err := addEnv(payload, r.Env); err != nil {
		return nil, err
	}
	return payload, nil
}

// UpdatePayload builds the PATCH /application/<id> body.
func UpdatePayload(r UpdateRequest) (map[string]any, error) {
	payload := map[string]any{}
	if len(r.Projects) > 0 {
		projects := make([]map[string]any, 0, len(r.Projects))
		for i, p := range r.Projects {
			project, err := projectPayload(p, false)
			if err != nil {
				return nil, fmt.Errorf("projects[%d]: %w", i, err)
			}
			projects = append(projects, project)
		}
		payload["projects"] = projects
	}
	if len(r.RemoveRepos) > 0 {
		payload["remove_repos"] = r.RemoveRepos
	}
	if err := addEnv(payload, r.Env); err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("nothing to update: set projects, remove_repos or env")
	}
	return payload, nil
}

func orgParams(c client.Client) map[string]string {
	params := make(map[string]string)
	if c.OrgLookupFn != nil {
		if org := c.OrgLookupFn(); org != "" {
			params["org"] = org
		}
	}
	return params
}

// Plan asks core what creating an app for repo ("owner/name") and branch
// would do. It changes nothing.
func Plan(c client.Client, repo, branch string) ([]byte, error) {
	return c.Requester.Do(
		http.MethodPost,
		uri.CreateResourceURI("", planResource, "", "", orgParams(c)),
		"application/json",
		map[string]string{"repo": repo, "branch": branch},
	)
}

// Create posts a CreatePayload body.
func Create(c client.Client, payload map[string]any) ([]byte, error) {
	return c.Requester.Do(
		http.MethodPost,
		uri.CreateResourceURI("", applicationResource, "", "", orgParams(c)),
		"application/json",
		payload,
	)
}

// Update patches the app with an UpdatePayload body.
func Update(c client.Client, id string, payload map[string]any) ([]byte, error) {
	return c.Requester.Do(
		http.MethodPatch,
		uri.CreateResourceURI("", applicationResource, id, "", orgParams(c)),
		"application/json",
		payload,
	)
}

// ActionBody returns the body of a failed request when core answered with a
// needs_action or already_exists status or a next_step, which the caller
// should show as an answer rather than a bare error.
func ActionBody(err error) ([]byte, bool) {
	apiErr, ok := requests.AsAPIError(err)
	if !ok {
		return nil, false
	}
	if !IsActionBody(apiErr.Body) {
		return nil, false
	}
	return apiErr.Body, true
}

// IsActionBody reports whether body carries a needs_action or already_exists
// status, or a next_step.
func IsActionBody(body []byte) bool {
	var r ActionResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return false
	}
	return r.Status == StatusNeedsAction || r.Status == StatusAlreadyExists || r.NextStep != nil
}

// RedactValues returns body with every env value replaced, so a response or
// error that echoes one does not reach the agent or the terminal. Objects with
// both name and value fields lose their value, and any string equal to one of
// the sent values is masked.
func RedactValues(body []byte, items []EnvItem) []byte {
	if len(body) == 0 {
		return body
	}
	values := sentValues(items)
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return []byte(RedactString(string(body), items))
	}
	if !redactJSON(v, values) {
		return body
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return []byte(RedactString(string(body), items))
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// RedactString masks every sent env value found in s. Values shorter than
// four characters ("1", "on") are left alone: masking them inside free text
// would garble the message without hiding anything secret.
func RedactString(s string, items []EnvItem) string {
	var values []string
	for v := range sentValues(items) {
		if len(v) >= 4 {
			values = append(values, v)
		}
	}
	// Longest first, so a value that contains another is masked whole
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, v := range values {
		s = strings.ReplaceAll(s, v, redacted)
	}
	return s
}

const redacted = "[REDACTED]"

func sentValues(items []EnvItem) map[string]bool {
	values := make(map[string]bool)
	for _, item := range items {
		if item.Value != "" {
			values[item.Value] = true
		}
	}
	return values
}

func redactJSON(v any, values map[string]bool) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		_, hasName := t["name"]
		for k, child := range t {
			if hasName && k == "value" {
				if child != redacted {
					t[k] = redacted
					changed = true
				}
				continue
			}
			if s, ok := child.(string); ok {
				if values[s] {
					t[k] = redacted
					changed = true
				}
				continue
			}
			if redactJSON(child, values) {
				changed = true
			}
		}
	case []any:
		for i, child := range t {
			if s, ok := child.(string); ok {
				if values[s] {
					t[i] = redacted
					changed = true
				}
				continue
			}
			if redactJSON(child, values) {
				changed = true
			}
		}
	}
	return changed
}
