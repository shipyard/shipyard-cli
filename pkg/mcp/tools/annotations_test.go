package tools

import "testing"

// An annotations entry for a tool that was renamed or removed is stale. The
// server test covers the other direction: every registered tool has one.
func TestAnnotationsNameDefinedTools(t *testing.T) {
	defined := map[string]bool{"get_logs": true} // LogsTool has no definitions map
	for _, defs := range []map[string]ToolDefinition{
		toolDefinitions, extendedToolDefinitions, orgToolDefinitions,
		telepresenceToolDefinitions, serviceToolDefinitions, volumeToolDefinitions, failureToolDefinitions,
	} {
		for name := range defs {
			defined[name] = true
		}
	}
	for name := range annotations {
		if !defined[name] {
			t.Errorf("annotations has an entry for %s, which is not a defined tool", name)
		}
	}
}
