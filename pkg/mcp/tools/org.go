package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/schemas"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/services/org"
)

// orgToolDefinitions maps tool names to their definitions
var orgToolDefinitions = map[string]ToolDefinition{
	"get_orgs": {
		Name:        "get_orgs",
		Description: "List all organizations that the user has access to",
		InputSchema: schemas.EmptySchema(),
	},
	"get_org": {
		Name:        "get_org",
		Description: "Get the currently configured default organization",
		InputSchema: schemas.EmptySchema(),
	},
	"set_org": {
		Name:        "set_org",
		Description: "Set the default organization in config",
		InputSchema: schemas.OrgNameSchema(),
	},
}

// OrgTool handles organization-related MCP operations
type OrgTool struct {
	client client.Client
	name   string
}

// NewOrgTool creates a new organization tool
func NewOrgTool(client client.Client, name string) *OrgTool {
	return &OrgTool{
		client: client,
		name:   name,
	}
}

// Definition returns the tool definition for MCP
func (t *OrgTool) Definition() ToolDefinition {
	if def, exists := orgToolDefinitions[t.name]; exists {
		return def
	}

	// Fallback for unknown tools
	return ToolDefinition{
		Name:        t.name,
		Description: "Unknown organization operation",
		InputSchema: map[string]interface{}{"type": "object"},
	}
}

// Execute runs the tool with given parameters
func (t *OrgTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	log.Printf("MCP tool execution started: %s with params: %s", t.name, string(params))
	fmt.Printf("DEBUG: MCP tool execution started: %s with params: %s\n", t.name, string(params))

	switch t.name {
	case "get_orgs":
		return t.executeGetOrgs(params)
	case "get_org":
		return t.executeGetOrg(params)
	case "set_org":
		return t.executeSetOrg(params)
	default:
		return "", fmt.Errorf("unknown operation: %s", t.name)
	}
}

func (t *OrgTool) executeGetOrgs(params json.RawMessage) (string, error) {
	// Call API directly to get raw JSON response (same as --json flag)
	body, err := t.client.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "org", "", "", nil), "application/json", nil)
	if err != nil {
		log.Printf("MCP get_orgs error: %v", err)
		return "", errors.ParseHTTPError("get_orgs", err, "")
	}

	// Return raw JSON response (same as --json flag output)
	return string(body), nil
}

func (t *OrgTool) executeGetOrg(params json.RawMessage) (string, error) {
	// Use service layer for business logic
	svc := org.NewOrganizationManager(t.client)
	currentOrg, err := svc.GetCurrent()
	if err != nil {
		log.Printf("MCP get_org error: %v", err)
		return "", errors.ParseHTTPError("get_org", err, "")
	}

	return fmt.Sprintf("Current organization: %s", currentOrg), nil
}

func (t *OrgTool) executeSetOrg(params json.RawMessage) (string, error) {
	var toolParams struct {
		OrgName string `json:"org_name"`
	}

	if err := json.Unmarshal(params, &toolParams); err != nil {
		return "", errors.ValidationError("set_org", "parameters", err.Error())
	}

	if toolParams.OrgName == "" {
		return "", errors.ValidationError("set_org", "org_name", "org_name parameter is required. Example: 'my-company', 'acme-corp'")
	}

	// Use service layer for business logic
	svc := org.NewOrganizationManager(t.client)
	err := svc.SetCurrent(toolParams.OrgName)
	if err != nil {
		log.Printf("MCP set_org error: %v", err)
		return "", errors.ParseHTTPError("set_org", err, toolParams.OrgName)
	}

	return fmt.Sprintf("Default organization set to: %s", toolParams.OrgName), nil
}
