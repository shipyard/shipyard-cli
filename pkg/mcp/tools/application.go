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

const nextStepNote = "If the result has status needs_action or already_exists, tell the user next_step.message " +
	"(and next_step.url, if any) and stop: do not retry until they have acted on it."

var applicationToolDefinitions = map[string]ToolDefinition{
	"plan_application": {
		Name: "plan_application",
		Description: "Preview what creating a Shipyard app for a pushed GitHub branch would do, without changing anything: " +
			"the compose file and its services, suggested routes (routes, route_candidates), the env var names it needs " +
			"(kind runtime or build_arg), head_sha, and any existing app for the repo. Call it before create_application " +
			"and confirm services, routes and env names with the user. " + nextStepNote,
		InputSchema: schemas.PlanApplicationSchema(),
	},
	"create_application": {
		Name: "create_application",
		Description: "Create a Shipyard app for a pushed GitHub branch, from a plan_application result the user confirmed. " +
			"Pass the plan's head_sha. Env values are sent to Shipyard but never returned: the result lists env names only. " +
			"Returns status created with the new app, or " + nextStepNote,
		InputSchema: schemas.CreateApplicationSchema(),
	},
	"update_application": {
		Name: "update_application",
		Description: "Reconfigure an existing app: change a repo's branch, services or routes, add or remove repos, or set " +
			"env vars. Only the fields given change. Material changes queue a rebuild. Env values are never returned. " +
			nextStepNote,
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

// Execute runs the named tool. Params of create and update carry env values,
// so they are never logged.
func (t *ApplicationTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	switch t.name {
	case "plan_application":
		log.Printf("MCP tool execution started: %s with params: %s", t.name, string(params))
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

type envParam struct {
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	Kind     string   `json:"kind,omitempty"`
	Hidden   *bool    `json:"hidden,omitempty"`
	Services []string `json:"services,omitempty"`
}

func toEnvItems(params []envParam) []apps.EnvItem {
	items := make([]apps.EnvItem, 0, len(params))
	for _, p := range params {
		items = append(items, apps.EnvItem(p))
	}
	return items
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
		Repo            string            `json:"repo"`
		Branch          string            `json:"branch"`
		AppName         string            `json:"app_name,omitempty"`
		Services        []string          `json:"services,omitempty"`
		Routes          map[string]string `json:"routes,omitempty"`
		Env             []envParam        `json:"env,omitempty"`
		HeadSHA         string            `json:"head_sha,omitempty"`
		ComposeFilename string            `json:"compose_filename,omitempty"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		// A JSON error can quote the input, which may hold a value
		return "", errors.ValidationError(t.name, "parameters", "arguments are not valid JSON for this tool")
	}
	if err := validateRepoBranch(t.name, toolParams.Repo, toolParams.Branch); err != nil {
		return "", err
	}

	env := toEnvItems(toolParams.Env)
	payload, err := apps.CreatePayload(apps.CreateRequest{
		AppName: toolParams.AppName,
		Project: apps.Project{
			Repo:            toolParams.Repo,
			Branch:          toolParams.Branch,
			Services:        toolParams.Services,
			Routes:          toolParams.Routes,
			ComposeFilename: toolParams.ComposeFilename,
		},
		Env:     env,
		HeadSHA: toolParams.HeadSHA,
	})
	if err != nil {
		return "", errors.ValidationError(t.name, "parameters", err.Error())
	}

	body, err := apps.Create(t.client, payload)
	return createdResult(body, err, env)
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
		RemoveRepos []string   `json:"remove_repos,omitempty"`
		Env         []envParam `json:"env,omitempty"`
	}
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError(t.name, "parameters", "arguments are not valid JSON for this tool")
	}
	if err := validation.ValidateEnvironmentID(toolParams.ApplicationID); err != nil {
		return "", errors.ValidationError(t.name, "application_id", err.Error())
	}

	req := apps.UpdateRequest{RemoveRepos: toolParams.RemoveRepos, Env: toEnvItems(toolParams.Env)}
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
	if actionBody, ok := apps.ActionBody(err); ok {
		return string(apps.RedactValues(actionBody, req.Env)), nil
	}
	if err != nil {
		return "", fmt.Errorf("%s", apps.RedactString(err.Error(), req.Env))
	}
	return successResult("updated", body, req.Env)
}

// createdResult turns core's create answer into the tool result: core's own
// needs_action or already_exists body when it sent one, else status created.
func createdResult(body []byte, err error, env []apps.EnvItem) (string, error) {
	if actionBody, ok := apps.ActionBody(err); ok {
		return string(apps.RedactValues(actionBody, env)), nil
	}
	if err != nil {
		return "", fmt.Errorf("%s", apps.RedactString(err.Error(), env))
	}
	// A 2xx can still carry a needs_action or already_exists status
	if apps.IsActionBody(body) {
		return string(apps.RedactValues(body, env)), nil
	}
	return successResult("created", body, env)
}

func successResult(status string, body []byte, env []apps.EnvItem) (string, error) {
	var resp struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Data) == 0 {
		resp.Data = json.RawMessage("null")
	}
	result := map[string]any{
		"status":      status,
		"application": json.RawMessage(apps.RedactValues(resp.Data, env)),
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
