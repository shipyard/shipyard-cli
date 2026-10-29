package tools

import "github.com/shipyard/shipyard-cli/pkg/client"

// NewHostedTool builds the tool a hosted server offers under name, using c for
// every API call. It returns nil for a tool HostedToolScopes doesn't list.
func NewHostedTool(c client.Client, name string) Tool {
	if _, ok := HostedToolScopes[name]; !ok {
		return nil
	}
	switch name {
	case "plan_application", "create_application", "update_application":
		return NewApplicationTool(c, name)
	case "get_environments", "get_environment":
		return NewEnvironmentTool(c, name)
	case "get_build_history", "get_failure_details", "get_build_logs":
		return NewExtendedTool(c, name)
	}
	return nil
}
