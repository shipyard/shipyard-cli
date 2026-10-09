package tools

// annotations holds the behavior hints for every MCP tool, keyed by tool name.
//
// destructiveHint follows the MCP spec: true unless the tool only adds state.
// Stopping, rebuilding or overwriting a running environment is not additive,
// even when it can be undone. openWorldHint is false for calls that only touch
// the user's own Shipyard resources, and true where the tool reaches past them:
// exec_service runs arbitrary commands, telepresence_connect joins the local
// machine to the cluster network.
var annotations = map[string]ToolAnnotations{
	// Environments
	"get_environments":    {Title: "List environments", ReadOnlyHint: true, IdempotentHint: true},
	"get_environment":     {Title: "Get environment", ReadOnlyHint: true, IdempotentHint: true},
	"restart_environment": {Title: "Restart environment", IdempotentHint: true},
	"stop_environment":    {Title: "Stop environment", DestructiveHint: true, IdempotentHint: true},
	"cancel_environment":  {Title: "Cancel environment build", DestructiveHint: true, IdempotentHint: true},
	"rebuild_environment": {Title: "Rebuild environment", DestructiveHint: true},
	"revive_environment":  {Title: "Revive environment", IdempotentHint: true},
	"deploy_detached":     {Title: "Deploy detached environment"},
	"update_branches":     {Title: "Update environment branches", DestructiveHint: true, IdempotentHint: true},
	"get_build_history":   {Title: "Get build history", ReadOnlyHint: true, IdempotentHint: true},

	// Organizations. set_org only rewrites the local CLI config.
	"get_orgs": {Title: "List organizations", ReadOnlyHint: true, IdempotentHint: true},
	"get_org":  {Title: "Get current organization", ReadOnlyHint: true, IdempotentHint: true},
	"set_org":  {Title: "Set current organization", IdempotentHint: true},

	// Services and logs. port_forward only returns the CLI command to run.
	"get_services":    {Title: "List services", ReadOnlyHint: true, IdempotentHint: true},
	"get_logs":        {Title: "Get service logs", ReadOnlyHint: true, IdempotentHint: true},
	"restart_service": {Title: "Restart service", DestructiveHint: true},
	"exec_service":    {Title: "Run command in service", DestructiveHint: true, OpenWorldHint: true},
	"port_forward":    {Title: "Port forward command", ReadOnlyHint: true, IdempotentHint: true},

	// Environment variables
	"get_env_vars":   {Title: "Get environment variables", ReadOnlyHint: true, IdempotentHint: true},
	"put_env_vars":   {Title: "Set environment variables", DestructiveHint: true, IdempotentHint: true},
	"delete_env_var": {Title: "Delete environment variable", DestructiveHint: true, IdempotentHint: true},

	// Volumes and snapshots
	"get_volumes":     {Title: "List volumes", ReadOnlyHint: true, IdempotentHint: true},
	"get_snapshots":   {Title: "List snapshots", ReadOnlyHint: true, IdempotentHint: true},
	"reset_volume":    {Title: "Reset volume", DestructiveHint: true, IdempotentHint: true},
	"create_snapshot": {Title: "Create snapshot"},
	"load_snapshot":   {Title: "Load snapshot", DestructiveHint: true, IdempotentHint: true},

	// Telepresence
	"telepresence_connect": {Title: "Connect with Telepresence", IdempotentHint: true, OpenWorldHint: true},
}

// Annotate returns def with its title and behavior hints attached. A tool with
// no entry is returned unchanged, so the server test that checks every
// registered tool catches it.
func Annotate(def ToolDefinition) ToolDefinition {
	a, ok := annotations[def.Name]
	if !ok {
		return def
	}
	def.Title = a.Title
	def.Annotations = &a
	return def
}
