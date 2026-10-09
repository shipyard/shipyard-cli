package tools

import (
	"strings"
	"testing"
)

// The server instructions no longer name get_orgs as the setup probe (they must fit
// Claude Code's 2048-char limit), and the verify prompt only runs when the user
// invokes it, so the tool description is what tells an agent to reach for it.
func TestOrgTool_GetOrgsDescriptionNamesTheSetupProbe(t *testing.T) {
	desc := NewOrgTool(newMockClient(), "get_orgs").Definition().Description
	for _, want := range []string{"no arguments", "broken setup", "does not exist yet"} {
		if !strings.Contains(desc, want) {
			t.Errorf("get_orgs description must say %q, got: %q", want, desc)
		}
	}
}
