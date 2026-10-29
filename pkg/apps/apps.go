// Package apps calls core's application create, reconfigure and plan APIs. The
// MCP tools and `shipyard add` share it, so both send the same bodies.
package apps

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf16"

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
	StatusNeedsAction   = "needs_action"
	StatusAlreadyExists = "already_exists"
	// StatusCreating: another request is still creating this app; nothing was
	// queued. Asking again shortly returns already_exists.
	StatusCreating = "creating"
)

// NextStep is what core says to do about a needs_action, already_exists or
// creating answer.
type NextStep struct {
	Kind    string `json:"kind"`
	URL     string `json:"url,omitempty"`
	Message string `json:"message"`
}

// EnvName is one variable the plan found, without a value.
type EnvName struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Services are the services the compose file gives the variable to; a
	// value set without them reaches every service. None means the file
	// interpolates it (${VAR}), so it is set for the whole app.
	Services []string `json:"services,omitempty"`
	// Optional is true when the compose file has a default for the variable.
	Optional bool `json:"optional,omitempty"`
}

// Route is a service's route in a plan.
type Route struct {
	Service string `json:"service"`
	Route   string `json:"route"`
}

// RouteCandidate is a service that could take the app's main route. Core may
// send a bare service name or an object with a service field.
type RouteCandidate string

// UnmarshalJSON accepts "web" or {"service": "web", ...}.
func (c *RouteCandidate) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err == nil {
		*c = RouteCandidate(name)
		return nil
	}
	var obj struct {
		Service string `json:"service"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	*c = RouteCandidate(obj.Service)
	return nil
}

// PlanService is a compose service in a plan. A service that runs only under a
// compose profile is off by default.
type PlanService struct {
	Name             string   `json:"name"`
	EnabledByDefault bool     `json:"enabled_by_default"`
	Profiles         []string `json:"profiles"`
}

// UnmarshalJSON accepts core's {"name": "web", "enabled_by_default": true, ...}
// or a bare "web", which is on by default.
func (s *PlanService) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err == nil {
		*s = PlanService{Name: name, EnabledByDefault: true}
		return nil
	}
	type plain PlanService
	return json.Unmarshal(b, (*plain)(s))
}

// PlanResponse is the body of POST /application/plan.
type PlanResponse struct {
	Status          string           `json:"status"`
	Org             string           `json:"org"`
	Repo            string           `json:"repo"`
	Branch          string           `json:"branch"`
	HeadSHA         string           `json:"head_sha"`
	ComposeFile     string           `json:"compose_file"`
	Services        []PlanService    `json:"services"`
	Routes          []Route          `json:"routes"`
	RouteCandidates []RouteCandidate `json:"route_candidates"`
	Env             []EnvName        `json:"env"`
	ExistingApp     json.RawMessage  `json:"existing_app,omitempty"`
	// AppName is a name core says create can take: free in the org, with a free
	// URL. Older cores don't send it.
	AppName string `json:"app_name,omitempty"`
	// VisitorsRequired is true for an org token, which has no user to let in.
	VisitorsRequired bool      `json:"visitors_required,omitempty"`
	NextStep         *NextStep `json:"next_step,omitempty"`
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
	// Routes maps a service to its shipyard.route label value. A nil value
	// removes the label.
	Routes map[string]*string
	// SelectedProfiles are the compose profiles to turn on. Core rejects an
	// enabled service whose profiles are all unselected.
	SelectedProfiles []string
}

// appNameChars is core's rule for an app name: letters, digits, dashes,
// underscores and spaces, at most maxAppNameLen of them.
const (
	appNameChars  = `- _a-zA-Z0-9`
	maxAppNameLen = 255
)

var (
	// appNameRE matches the characters core does not allow in an app name.
	appNameRE  = regexp.MustCompile(`[^` + appNameChars + `]+`)
	validAppRE = regexp.MustCompile(`^[` + appNameChars + `]+$`)
)

// ValidAppName reports whether core accepts name as an app name.
func ValidAppName(name string) bool {
	return len(name) <= maxAppNameLen && validAppRE.MatchString(name)
}

// DefaultAppName is "<repo name>-<branch>", since app names are unique per org
// and another branch of the repo may already have an app. Characters core
// rejects become dashes, and then a short hash of the original keeps
// feature/x and feature-x apart; the name is cut to core's length limit.
func DefaultAppName(repo, branch string) string {
	_, name, _ := SplitRepo(repo)
	if branch != "" {
		name += "-" + branch
	}
	clean := appNameRE.ReplaceAllString(name, "-")
	suffix := ""
	if clean != name || len(clean) > maxAppNameLen {
		sum := sha256.Sum256([]byte(name))
		suffix = "-" + hex.EncodeToString(sum[:3])
	}
	if len(clean)+len(suffix) > maxAppNameLen {
		clean = clean[:maxAppNameLen-len(suffix)]
	}
	return clean + suffix
}

// CreateRequest is the input of Create.
type CreateRequest struct {
	AppName string
	Project Project
	Env     []EnvItem
	// HeadSHA is the commit the plan saw, so core can refuse a stale plan.
	HeadSHA string
	// Visitors are GitHub logins who may open the environment. An org API
	// token has no user to add, so core requires them with one.
	Visitors []string
}

// UpdateRequest is the input of Update. Only the fields set are changed. Env
// vars are not among them: core's PATCH ignores secrets and build_args.
type UpdateRequest struct {
	Projects    []Project
	RemoveRepos []string
}

// PublicValues lists what a request sends in the clear: repo, branch, app
// name, services and routes. A sent env value equal to one is no secret, and
// masking it would garble service names and links in core's answer.
func PublicValues(appName string, projects ...Project) []string {
	public := []string{appName}
	for _, p := range projects {
		public = append(public, p.Repo, p.Branch, p.ComposeFilename)
		if owner, name, err := SplitRepo(p.Repo); err == nil {
			public = append(public, owner, name)
		}
		public = append(public, p.Services...)
		for svc, route := range p.Routes {
			public = append(public, svc)
			if route != nil {
				public = append(public, *route)
			}
		}
	}
	return public
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
	// nil leaves the profiles alone; an empty list turns them all off
	if p.SelectedProfiles != nil {
		out["selectedProfiles"] = p.SelectedProfiles
	}
	if len(p.Routes) > 0 {
		labels := make(map[string]any, len(p.Routes))
		for svc, route := range p.Routes {
			var value any // null removes the label
			if route != nil {
				value = *route
			}
			labels[svc] = map[string]any{RouteLabel: value}
		}
		out["serviceLabels"] = labels
	}
	return out, nil
}

// addEnv splits env items into core's secrets and build_args lists.
func addEnv(payload map[string]any, items []EnvItem) error {
	var secrets, buildArgs []map[string]any
	for i, v := range items {
		if v.Name == "" {
			return fmt.Errorf("env[%d].name is required", i)
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
			return fmt.Errorf("env[%d].kind must be %s or %s, got %q", i, KindRuntime, KindBuildArg, v.Kind)
		}
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
	if len(r.Visitors) > 0 {
		payload["visitors"] = r.Visitors
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
		// Core matches linked repos by name only and never sees an owner, so
		// dropping one could unlink another owner's repo of the same name
		for _, repo := range r.RemoveRepos {
			owner, _, err := SplitRepo(repo)
			if err != nil {
				return nil, fmt.Errorf("remove_repos: %w", err)
			}
			if owner != "" {
				return nil, fmt.Errorf("remove_repos: pass the bare repo name as the app links it, not %q", repo)
			}
		}
		payload["remove_repos"] = r.RemoveRepos
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("nothing to update: set projects or remove_repos")
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

// NameTaken reports whether a live environment in the org already uses name,
// which core refuses on create. The listing is cached and covers only the
// environment types it shows, so core's own check stays authoritative.
func NameTaken(c client.Client, name string) (bool, error) {
	params := orgParams(c)
	params["name"] = name
	params["page_size"] = "1"
	body, err := c.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "environment", "", "", params), "application/json", nil)
	if err != nil {
		return false, err
	}
	var list struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return false, fmt.Errorf("reading environments: %w", err)
	}
	return len(list.Data) > 0, nil
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
// needs_action, already_exists or creating status or a next_step, which the caller
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

// IsActionBody reports whether body carries a needs_action, already_exists or
// creating status, or a next_step.
func IsActionBody(body []byte) bool {
	var r ActionResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return false
	}
	return r.Status == StatusNeedsAction || r.Status == StatusAlreadyExists || r.Status == StatusCreating ||
		r.NextStep != nil
}

// RedactValues returns body with every env value replaced, so a response or
// error that echoes one does not reach the agent or the terminal. Objects with
// both name and value fields lose their value, a string or number equal to a
// sent value is masked, and a sent value inside a longer string is masked as
// RedactString does.
//
// public lists values the request sent in the clear (see PublicValues). An env
// value equal to one of them is left alone everywhere.
func RedactValues(body []byte, items []EnvItem, public ...string) []byte {
	if len(body) == 0 {
		return body
	}
	r := newRedactor(items, public)
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	// Anything after the first JSON value goes unread, as in a plain-text
	// "400 bad request: ..." that decodes as the number 400: mask it as text
	if err := dec.Decode(&v); err != nil || len(bytes.TrimSpace(body[dec.InputOffset():])) > 0 {
		return []byte(r.mask(string(body)))
	}
	// A bare JSON string or number has no object to walk
	if m, ok := r.leaf(v); ok {
		v = m
	} else if !r.redactJSON(v) {
		return body
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return []byte(r.mask(string(body)))
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// RedactString masks every sent env value found in s, including its
// JSON-escaped form, which is how a value with a newline or quote appears in a
// raw error body. Values shorter than minSubstringLen are left alone inside
// text: masking them would garble the message without hiding anything secret.
func RedactString(s string, items []EnvItem, public ...string) string {
	return newRedactor(items, public).mask(s)
}

// RedactError returns err with every env value masked. An API error is rebuilt
// from its redacted body, so values are masked before the message is shaped:
// the title's first letter is lower-cased and JSON escapes are decoded only
// after redaction, and neither can hide a value from it.
func RedactError(err error, items []EnvItem, public ...string) error {
	if err == nil {
		return nil
	}
	if apiErr, ok := requests.AsAPIError(err); ok {
		return requests.NewAPIError(apiErr.StatusCode, RedactValues(apiErr.Body, items, public...))
	}
	return errors.New(RedactString(err.Error(), items, public...))
}

const redacted = "[REDACTED]"

type redactor struct {
	// exact holds every sent value, matched against whole JSON leaves.
	exact map[string]bool
	// patterns holds the values masked inside longer text and their
	// JSON-escaped forms, all in one list longest first: a value that contains
	// another value, or another value's escaped form, is masked whole.
	patterns []string
}

// minSubstringLen is the shortest value masked inside longer text. Shorter
// values are everyday words ("https", "8080", "true") that would garble URLs
// and ids without hiding anything secret; a whole field equal to one is still
// masked.
const minSubstringLen = 8

// escapedForms are the ways a JSON encoder can write v inside a string: Go's
// (non-ASCII kept) and Python's default (non-ASCII as \uXXXX), which core uses.
func escapedForms(v string) []string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil
	}
	b := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	plain := string(b[1 : len(b)-1])
	var ascii strings.Builder
	for _, r := range plain {
		switch {
		case r < 0x80:
			ascii.WriteRune(r)
		case r > 0xFFFF:
			r1, r2 := utf16.EncodeRune(r)
			fmt.Fprintf(&ascii, "\\u%04x\\u%04x", r1, r2)
		default:
			fmt.Fprintf(&ascii, "\\u%04x", r)
		}
	}
	return []string{plain, ascii.String()}
}

func newRedactor(items []EnvItem, public []string) redactor {
	r := redactor{exact: make(map[string]bool)}
	clear := make(map[string]bool, len(public))
	for _, p := range public {
		clear[p] = true
	}
	for _, item := range items {
		v := item.Value
		if v == "" || r.exact[v] || clear[v] {
			continue
		}
		r.exact[v] = true
		if len(v) < minSubstringLen {
			continue
		}
		for _, form := range append([]string{v}, escapedForms(v)...) {
			if !slices.Contains(r.patterns, form) {
				r.patterns = append(r.patterns, form)
			}
		}
	}
	sort.SliceStable(r.patterns, func(i, j int) bool { return len(r.patterns[i]) > len(r.patterns[j]) })
	return r
}

// mask replaces the long values in s and their escaped forms. A decoded JSON
// string can still hold an escaped form when it carries serialized JSON.
func (r redactor) mask(s string) string {
	for _, v := range r.patterns {
		s = strings.ReplaceAll(s, v, redacted)
	}
	return s
}

// leaf returns the masked form of a JSON leaf, and whether it changed.
func (r redactor) leaf(v any) (any, bool) {
	switch t := v.(type) {
	case string:
		if r.exact[t] {
			return redacted, true
		}
		if m := r.mask(t); m != t {
			return m, true
		}
	case json.Number:
		if r.exact[t.String()] {
			return redacted, true
		}
	}
	return v, false
}

func (r redactor) redactJSON(v any) bool {
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
			if m, ok := r.leaf(child); ok {
				t[k] = m
				changed = true
			} else if r.redactJSON(child) {
				changed = true
			}
		}
		// A key can carry a value too, as in {"errors": {"<value>": "invalid"}}
		for _, k := range slices.Collect(maps.Keys(t)) {
			if m, ok := r.leaf(k); ok {
				key := m.(string)
				for _, taken := t[key]; taken; _, taken = t[key] {
					key += "_"
				}
				t[key] = t[k]
				delete(t, k)
				changed = true
			}
		}
	case []any:
		for i, child := range t {
			if m, ok := r.leaf(child); ok {
				t[i] = m
				changed = true
			} else if r.redactJSON(child) {
				changed = true
			}
		}
	}
	return changed
}
