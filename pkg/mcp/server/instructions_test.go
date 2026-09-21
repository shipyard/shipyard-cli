package server

import (
	"strings"
	"testing"
)

func TestInstructionsNotEmpty(t *testing.T) {
	got := Instructions()

	if got == "" {
		t.Fatal("expected server instructions, got empty string")
	}
	if strings.HasPrefix(got, "\n") || strings.HasSuffix(got, "\n") {
		t.Error("expected instructions to be trimmed")
	}
}

// The instructions land in every session for every client that surfaces them, so
// they must stay a summary. The full loop lives in the shipyard_verify prompt.
func TestInstructionsStaySmall(t *testing.T) {
	const budget = 2500

	if size := len(Instructions()); size > budget {
		t.Errorf("instructions are %d chars, over the %d budget: move detail into the shipyard_verify prompt", size, budget)
	}
}

// Each of these is a rule an agent gets wrong by default, verified against a live
// environment. Losing one silently would ship instructions that read fine and let
// an agent report a false pass.
func TestInstructionsCoverTheTraps(t *testing.T) {
	body := Instructions()

	traps := map[string]string{
		"ready":               "the commit lands before the environment serves it",
		"stopped":             "stopped environments never become ready",
		"retired":             "retired environments never become ready",
		"rebuild_environment": "never rebuild while a build is in flight",
		"bypass_token":        "how to reach the environment",
		"repo_name":           "match the right project in a multi-repo environment",
		"shipyard_verify":     "where the full loop lives",
	}

	for needle, why := range traps {
		if !strings.Contains(body, needle) {
			t.Errorf("instructions no longer mention %q (%s)", needle, why)
		}
	}
}
