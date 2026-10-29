package tools

// OAuth scopes a hosted MCP server grants. A hosted token carries some of these,
// and a tool runs only when the token has the tool's scope.
const (
	ScopeAppsCreate        = "apps:create"
	ScopeAppsUpdate        = "apps:update"
	ScopeEnvsRead          = "envs:read"
	ScopeEnvsPreviewAccess = "envs:preview_access"
)

// KnownScopes lists every scope a tool may require.
var KnownScopes = []string{ScopeAppsCreate, ScopeAppsUpdate, ScopeEnvsRead, ScopeEnvsPreviewAccess}

// HostedToolScopes is the one table of which tools a hosted server offers and
// the scope each requires. A tool missing from it is never offered hosted.
// Local-only tools (exec_service, port_forward, telepresence_connect) reach the
// user's machine or the cluster directly and must never be added.
var HostedToolScopes = map[string]string{
	// Planning reads the repo only to set up a create.
	"plan_application":   ScopeAppsCreate,
	"create_application": ScopeAppsCreate,
	"update_application": ScopeAppsUpdate,

	"get_environments":    ScopeEnvsRead,
	"get_environment":     ScopeEnvsRead,
	"get_build_history":   ScopeEnvsRead,
	"get_failure_details": ScopeEnvsRead,
	"get_build_logs":      ScopeEnvsRead,
}

// localOnlyTools are never offered by a hosted server.
var localOnlyTools = []string{"exec_service", "port_forward", "telepresence_connect"}
