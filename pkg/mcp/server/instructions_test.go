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
// they must stay a summary. The full loop lives in the verify prompt.
//
// The budget is a hard limit, not a style preference: Claude Code (2.1.284,
// measured 2026-10-09) cuts server instructions off at 2048 characters, and
// everything past that never reaches the model. At 2463 characters the last
// rules, including never printing a bypass_token, were being dropped. The
// budget leaves about 120 characters for one leading notice line, such as the
// out-of-date CLI notice, under that limit. len counts bytes, which is stricter
// than the client's character count.
func TestInstructionsStaySmall(t *testing.T) {
	const budget = 1930

	if size := len(Instructions()); size > budget {
		t.Errorf("instructions are %d chars, over the %d budget: move detail into the verify prompt", size, budget)
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
		"never print":         "a bypass_token must not land in chat, a commit or a pull request",
		"repo_name":           "match the right project in a multi-repo environment",
		"The `verify` prompt": "where the full loop lives",
	}

	for needle, why := range traps {
		if !strings.Contains(body, needle) {
			t.Errorf("instructions no longer mention %q (%s)", needle, why)
		}
	}
}
