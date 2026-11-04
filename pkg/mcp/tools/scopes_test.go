package tools

import (
	"slices"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/client"
)

func TestHostedToolScopes(t *testing.T) {
	defined := map[string]bool{"get_logs": true} // LogsTool has no definitions map
	for _, defs := range []map[string]ToolDefinition{
		toolDefinitions, extendedToolDefinitions, orgToolDefinitions,
		telepresenceToolDefinitions, serviceToolDefinitions, volumeToolDefinitions,
		applicationToolDefinitions,
	} {
		for name := range defs {
			defined[name] = true
		}
	}

	for name, scope := range HostedToolScopes {
		if !defined[name] {
			t.Errorf("HostedToolScopes has %s, which is not a defined tool", name)
		}
		if scope == "" {
			t.Errorf("hosted tool %s has no scope", name)
		} else if !slices.Contains(KnownScopes, scope) {
			t.Errorf("hosted tool %s requires unknown scope %q", name, scope)
		}
	}
	for _, name := range localOnlyTools {
		if _, ok := HostedToolScopes[name]; ok {
			t.Errorf("%s is local-only and must not be offered hosted", name)
		}
	}
	// The app tools and the env read tools make up the hosted set.
	hosted := []string{"get_environments", "get_environment", "get_build_history", "get_failure_details", "get_build_logs"}
	for name := range applicationToolDefinitions {
		hosted = append(hosted, name)
	}
	for _, name := range hosted {
		if _, ok := HostedToolScopes[name]; !ok {
			t.Errorf("hosted tool %s has no scope", name)
		}
	}
}

func TestNewHostedToolBuildsEveryHostedTool(t *testing.T) {
	for name := range HostedToolScopes {
		tool := NewHostedTool(client.Client{}, name)
		if tool == nil {
			t.Errorf("NewHostedTool(%s) = nil", name)
			continue
		}
		if got := tool.Definition().Name; got != name {
			t.Errorf("NewHostedTool(%s) built %s", name, got)
		}
	}
	for _, name := range append(localOnlyTools, "put_env_vars", "stop_environment") {
		if NewHostedTool(client.Client{}, name) != nil {
			t.Errorf("NewHostedTool(%s) built a tool that isn't hosted", name)
		}
	}
}
