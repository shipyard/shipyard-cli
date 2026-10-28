package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/shipyard/shipyard-cli/pkg/apps"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/schemas"
	"github.com/shipyard/shipyard-cli/pkg/mcp/validation"
)

const nextStepNote = "If the result has status needs_action, tell the user next_step.message " +
	"(and next_step.url, if any) and stop: do not retry until they have acted on it. " +
	"Status already_exists means an app is already on this repo and branch: data is that app and " +
	"requested_vs_existing lists what this request asked for that it lacks; offer update_application for services " +
	"and routes, and put_env_vars for env vars. " +
	"Status creating means another request is still creating this app: wait about 30 seconds and call " +
	"create_application again with the same input."

// bypassTokenNote is the rule get_environment carries too: the app in the
// result can include its bypass_token.
const bypassTokenNote = "The app in the result can carry a bypass_token, which gets past its login gate: " +
	"never print it or paste it into chat, a commit, or a pull request comment. "

var applicationToolDefinitions = map[string]ToolDefinition{
	"plan_application": {
		Name: "plan_application",
		Description: "Preview what creating a Shipyard app for a pushed GitHub branch would do, without changing anything: " +
			"the compose file and its services, suggested routes (routes, route_candidates), the env var names it needs " +
			"(kind runtime or build_arg, and the services that use each), head_sha, and any existing app for the repo. " +
			"Call it before create_application and confirm services, routes and env names with the user. " + nextStepNote,
		InputSchema: schemas.PlanApplicationSchema(),
	},
	"create_application": {
		Name: "create_application",
		Description: "Create a Shipyard app for a pushed GitHub branch, from a plan_application result the user confirmed. " +
			"head_sha is required: pass the plan's, so Shipyard builds the commit the user confirmed. " +
			"Env values are sent to Shipyard but never returned: the result lists env names only. " +
			"Set each env item's services to the plan's, so a value reaches only the services that use it. " +
			"Enabling a service whose enabled_by_default is false needs its profile in selected_profiles. " +
			bypassTokenNote + "Returns status created with the new app, or " + nextStepNote,
		InputSchema: schemas.CreateApplicationSchema(),
	},
	"update_application": {
		Name: "update_application",
		Description: "Reconfigure an existing app: change a repo's branch, services or routes, or add or remove repos. " +
			"Only the fields given change. Material changes queue a rebuild. It does not set env vars: use put_env_vars. " +
			bypassTokenNote + nextStepNote,
		InputSchema: schemas.UpdateApplicationSchema(),
	},
}

// ApplicationTool plans, creates and reconfigures apps.
type ApplicationTool struct {
	client client.Client
	name   string
}

// NewApplicationTool creates an application MCP tool for the given name.
func NewApplicationTool(client client.Client, name string) *ApplicationTool {
	return &ApplicationTool{client: client, name: name}
}

// Definition returns the MCP tool definition.
func (t *ApplicationTool) Definition() ToolDefinition {
	if def, ok := applicationToolDefinitions[t.name]; ok {
		return def
	}
	return ToolDefinition{
		Name:        t.name,
		Description: "Unknown operation",
		InputSchema: map[string]interface{}{"type": "object"},
	}
}

// Execute runs the named tool. Params are never logged: create's carry env
// values, and an agent may send them to plan or update by mistake.
func (t *ApplicationTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	switch t.name {
	case "plan_application":
		log.Printf("MCP tool execution started: %s", t.name)
		return t.executePlan(params)
	case "create_application":
		log.Printf("MCP tool execution started: %s", t.name)
		return t.executeCreate(params)
	case "update_application":
		log.Printf("MCP tool execution started: %s", t.name)
		return t.executeUpdate(params)
	default:
		return "", fmt.Errorf("unknown operation: %s", t.name)
	}
}

// requireEnvValues rejects an env item whose value is missing, null or empty.
// The schema requires it, but nothing enforces the schema, and an empty value
// would clear a working variable. delete_env_var removes one on purpose.
func requireEnvValues(tool string, params json.RawMessage) error {
	var raw struct {
		Env []struct {
			Value *string `json:"value"`
		} `json:"env"`
	}
	if err := json.Unmarshal(params, &raw); err != nil {
		return errors.ValidationError(tool, "parameters", "arguments are not valid JSON for this tool")
	}
	for i, item := range raw.Env {
		if item.Value == nil || *item.Value == "" {
			return errors.ValidationError(tool, "env", fmt.Sprintf("env[%d].value is required", i))
		}
	}
	return nil
}

// envNames lists what was set, for the result: names and kinds, never values.
func envNames(items []apps.EnvItem) []apps.EnvName {
	names := make([]apps.EnvName, 0, len(items))
	for _, item := range items {
		kind := item.Kind
		if kind == "" {
			kind = apps.KindRuntime
		}
		names = append(names, apps.EnvName{Name: item.Name, Kind: kind})
	}
	return names
}

func (t *ApplicationTool) executePlan(params json.RawMessage) (string, error) {
	var toolParams struct {
		Repo   string `json:"repo"`
		Branch string `json:"branch"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError(t.name, "parameters", err.Error())
	}
	if err := validateRepoBranch(t.name, toolParams.Repo, toolParams.Branch); err != nil {
		return "", err
	}

	body, err := apps.Plan(t.client, toolParams.Repo, toolParams.Branch)
	if actionBody, ok := apps.ActionBody(err); ok {
		return string(actionBody), nil
	}
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func validateRepoBranch(tool, repo, branch string) error {
	owner, _, err := apps.SplitRepo(repo)
	if err == nil && owner == "" {
		err = fmt.Errorf("repo must be owner/name, got %q", repo)
	}
	if err == nil {
		err = validation.ValidateRepoName(repo)
	}
	if err != nil {
		return errors.ValidationError(tool, "repo", err.Error())
	}
	if branch == "" {
		return errors.ValidationError(tool, "branch", "branch is required")
	}
	if err := validation.ValidateBranchName(branch); err != nil {
		return errors.ValidationError(tool, "branch", err.Error())
	}
	return nil
}

func (t *ApplicationTool) executeCreate(params json.RawMessage) (string, error) {
	var toolParams struct {
		Repo             string            `json:"repo"`
		Branch           string            `json:"branch"`
		AppName          string            `json:"app_name,omitempty"`
		Services         []string          `json:"services,omitempty"`
		Routes           map[string]string `json:"routes,omitempty"`
		Env              []apps.EnvItem    `json:"env,omitempty"`
		HeadSHA          string            `json:"head_sha,omitempty"`
		ComposeFilename  string            `json:"compose_filename,omitempty"`
		Visitors         []string          `json:"visitors,omitempty"`
		SelectedProfiles []string          `json:"selected_profiles,omitempty"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		// A JSON error can quote the input, which may hold a value
		return "", errors.ValidationError(t.name, "parameters", "arguments are not valid JSON for this tool")
	}
	if err := validateRepoBranch(t.name, toolParams.Repo, toolParams.Branch); err != nil {
		return "", err
	}
	// Without the plan's commit, core would build whatever the branch holds
	// now, which may be code pushed after the user confirmed the plan
	if toolParams.HeadSHA == "" {
		return "", errors.ValidationError(t.name, "head_sha", "head_sha is required: pass the head_sha from plan_application")
	}
	if err := requireEnvValues(t.name, params); err != nil {
		return "", err
	}
	// Core enables no service by default and rejects a create without one
	if len(toolParams.Services) == 0 {
		return "", errors.ValidationError(t.name, "services", "services is required: pass the plan's services to enable")
	}
	// Core rejects a create without a name
	appName := toolParams.AppName
	if appName == "" {
		appName = apps.DefaultAppName(toolParams.Repo)
	}

	env := toolParams.Env
	project := apps.Project{
		Repo:             toolParams.Repo,
		Branch:           toolParams.Branch,
		Services:         toolParams.Services,
		Routes:           toolParams.Routes,
		ComposeFilename:  toolParams.ComposeFilename,
		SelectedProfiles: toolParams.SelectedProfiles,
	}
	payload, err := apps.CreatePayload(apps.CreateRequest{
		AppName:  appName,
		Project:  project,
		Env:      env,
		HeadSHA:  toolParams.HeadSHA,
		Visitors: toolParams.Visitors,
	})
	if err != nil {
		return "", errors.ValidationError(t.name, "parameters", err.Error())
	}

	body, err := apps.Create(t.client, payload)
	return actionOrSuccess("created", body, err, env, apps.PublicValues(appName, project))
}

func (t *ApplicationTool) executeUpdate(params json.RawMessage) (string, error) {
	var toolParams struct {
		ApplicationID string `json:"application_id"`
		Projects      []struct {
			Repo            string            `json:"repo"`
			Branch          string            `json:"branch,omitempty"`
			Services        []string          `json:"services,omitempty"`
			Routes          map[string]string `json:"routes,omitempty"`
			ComposeFilename string            `json:"compose_filename,omitempty"`
		} `json:"projects,omitempty"`
		RemoveRepos []string `json:"remove_repos,omitempty"`
		// Env is read only to refuse it: core's PATCH ignores env vars, so
		// accepting them would report a change that never happened
		Env json.RawMessage `json:"env,omitempty"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError(t.name, "parameters", "arguments are not valid JSON for this tool")
	}
	if err := validation.ValidateApplicationID(toolParams.ApplicationID); err != nil {
		return "", errors.ValidationError(t.name, "application_id", err.Error())
	}
	if len(toolParams.Env) > 0 {
		return "", errors.ValidationError(t.name, "env", "update_application does not set env vars: use put_env_vars")
	}

	req := apps.UpdateRequest{RemoveRepos: toolParams.RemoveRepos}
	for i, p := range toolParams.Projects {
		if err := validation.ValidateRepoName(p.Repo); err != nil {
			return "", errors.ValidationError(t.name, "projects", fmt.Sprintf("projects[%d]: %v", i, err))
		}
		if err := validation.ValidateBranchName(p.Branch); err != nil {
			return "", errors.ValidationError(t.name, "projects", fmt.Sprintf("projects[%d]: %v", i, err))
		}
		req.Projects = append(req.Projects, apps.Project{
			Repo:            p.Repo,
			Branch:          p.Branch,
			Services:        p.Services,
			Routes:          p.Routes,
			ComposeFilename: p.ComposeFilename,
		})
	}
	payload, err := apps.UpdatePayload(req)
	if err != nil {
		return "", errors.ValidationError(t.name, "parameters", err.Error())
	}

	body, err := apps.Update(t.client, toolParams.ApplicationID, payload)
	return actionOrSuccess("updated", body, err, nil, nil)
}

// actionOrSuccess turns core's create or update answer into the tool result:
// core's own needs_action, already_exists or creating body when it sent one,
// else the given success status. public lists what the request sent in the
// clear (apps.PublicValues), which redaction leaves alone.
func actionOrSuccess(status string, body []byte, err error, env []apps.EnvItem, public []string) (string, error) {
	if actionBody, ok := apps.ActionBody(err); ok {
		return string(apps.RedactValues(actionBody, env, public...)), nil
	}
	if err != nil {
		return "", apps.RedactError(err, env, public...)
	}
	// A 2xx can still carry a needs_action, already_exists or creating status
	if apps.IsActionBody(body) {
		return string(apps.RedactValues(body, env, public...)), nil
	}
	return successResult(status, body, env, public)
}

func successResult(status string, body []byte, env []apps.EnvItem, public []string) (string, error) {
	var resp struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Data) == 0 {
		resp.Data = json.RawMessage("null")
	}
	result := map[string]any{
		"status":      status,
		"application": json.RawMessage(apps.RedactValues(resp.Data, env, public...)),
	}
	if len(env) > 0 {
		result["env"] = envNames(env)
	}
	out, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
