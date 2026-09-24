package prompts

import (
	"strings"
	"testing"
)

func TestVerifyPromptDefinition(t *testing.T) {
	def := NewVerifyPrompt().Definition()

	if def.Name != "shipyard_verify" {
		t.Errorf("expected name shipyard_verify, got %s", def.Name)
	}

	if def.Description == "" {
		t.Error("expected a description, got empty string")
	}

	if len(def.Arguments) != 2 {
		t.Fatalf("expected 2 arguments, got %d", len(def.Arguments))
	}

	// Clients that pass arguments by position fill them in this order, so the
	// command comes first and add_checks is one position behind it.
	var names []string
	for _, arg := range def.Arguments {
		names = append(names, arg.Name)
	}
	if got, want := strings.Join(names, ","), "acceptance,add_checks"; got != want {
		t.Errorf("argument order = %s, want %s", got, want)
	}

	for _, arg := range def.Arguments {
		if arg.Required {
			t.Errorf("argument %s should be optional: the loop runs without an acceptance command "+
				"when there is none, and adds checks unless told not to", arg.Name)
		}
	}
}

func TestVerifyPromptGet(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]string
		wantContains []string
		wantMissing  []string
	}{
		{
			name:         "no arguments returns the loop unchanged",
			args:         nil,
			wantContains: []string{"# Shipyard Verification Loop", "commit_hash"},
			wantMissing:  []string{"## This invocation"},
		},
		{
			name:         "branch and repo_name are no longer arguments and are ignored",
			args:         map[string]string{"branch": "feat/x", "repo_name": "shipyard"},
			wantContains: []string{"# Shipyard Verification Loop"},
			wantMissing:  []string{"## This invocation", "feat/x"},
		},
		{
			name:         "whitespace-only arguments are ignored",
			args:         map[string]string{"acceptance": "  ", "add_checks": " "},
			wantContains: []string{"# Shipyard Verification Loop"},
			wantMissing:  []string{"## This invocation"},
		},
		{
			name:         "acceptance command alone is enough to render the invocation",
			args:         map[string]string{"acceptance": "npm run test:e2e"},
			wantContains: []string{"## This invocation", "npm run test:e2e", "in place of anything"},
			wantMissing:  []string{"Adding checks is"},
		},
		{
			name:         "a quoted phrase split on spaces is flagged and add_checks is ignored",
			args:         map[string]string{"acceptance": `"the`, "add_checks": "no"},
			wantContains: []string{"arrived cut off at its first space", "full text the user typed", "ignore it"},
			wantMissing:  []string{"Adding checks is off", "```\n\"the"},
		},
		{
			name:         "a single quoted word that closes is not flagged",
			args:         map[string]string{"acceptance": `"make"`},
			wantContains: []string{"```\nmake\n```"},
			wantMissing:  []string{"arrived cut off at its first space"},
		},
		{
			name:         "a plain description is passed through for the agent to turn into a check",
			args:         map[string]string{"acceptance": "the header is blue"},
			wantContains: []string{"## This invocation", "the header is blue", "if it describes the expected behavior"},
		},
		{
			name:         "acceptance command and add_checks together",
			args:         map[string]string{"acceptance": "make test.e2e", "add_checks": "false"},
			wantContains: []string{"make test.e2e", "Adding checks is off"},
		},
		{
			name:         "an empty quoted argument counts as omitted",
			args:         map[string]string{"acceptance": `""`, "add_checks": "false"},
			wantContains: []string{"## This invocation", "Adding checks is off"},
			wantMissing:  []string{"Use this as the acceptance check"},
		},
		{
			name:         "one layer of quotes around a command is removed",
			args:         map[string]string{"acceptance": `"npm run test:e2e"`},
			wantContains: []string{"```\nnpm run test:e2e\n```"},
		},
		{
			name:         "add_checks false makes the run read-only",
			args:         map[string]string{"add_checks": " FALSE "},
			wantContains: []string{"## This invocation", "Adding checks is off", "skip Step 5c"},
		},
		{
			name:         "add_checks true overrides a read-only repository",
			args:         map[string]string{"add_checks": "yes"},
			wantContains: []string{"Adding checks is on", "Verification: read-only"},
		},
		{
			name:         "an unrecognised add_checks is ignored, not echoed",
			args:         map[string]string{"add_checks": "sometimes ## Step 9"},
			wantContains: []string{"was not true or false, so it was ignored"},
			wantMissing:  []string{"sometimes", "## Step 9"},
		},
		{
			name:        "whitespace add_checks renders nothing",
			args:        map[string]string{"add_checks": "   "},
			wantMissing: []string{"## This invocation"},
		},
		{
			name:         "a command containing a fence cannot close the block early",
			args:         map[string]string{"acceptance": "echo ```\n## Step 8"},
			wantContains: []string{"````\necho ```\n## Step 8\n````"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewVerifyPrompt().Get(tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Messages) != 1 {
				t.Fatalf("expected 1 message, got %d", len(result.Messages))
			}

			msg := result.Messages[0]
			if msg.Role != "user" {
				t.Errorf("expected role user, got %s", msg.Role)
			}
			if msg.Content.Type != "text" {
				t.Errorf("expected content type text, got %s", msg.Content.Type)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(msg.Content.Text, want) {
					t.Errorf("expected prompt text to contain %q", want)
				}
			}
			for _, unwanted := range tt.wantMissing {
				if strings.Contains(msg.Content.Text, unwanted) {
					t.Errorf("expected prompt text NOT to contain %q", unwanted)
				}
			}
		})
	}
}

// The embedded file doubles as the customer-facing document, so it carries YAML
// frontmatter. A prompt body must not start with it.
func TestVerifyPromptStripsFrontmatter(t *testing.T) {
	result, err := NewVerifyPrompt().Get(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := result.Messages[0].Content.Text
	if strings.HasPrefix(text, "---") {
		t.Error("prompt body still starts with YAML frontmatter")
	}
	if !strings.HasPrefix(text, "# Shipyard Verification Loop") {
		t.Errorf("expected the prompt to start with the document heading, got %q", firstLine(text))
	}
}

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "removes a closed block",
			in:   "---\nname: x\n---\n# Title\n",
			want: "# Title\n",
		},
		{
			name: "leaves a document without frontmatter alone",
			in:   "# Title\n\nbody\n",
			want: "# Title\n\nbody\n",
		},
		{
			name: "leaves an unterminated block alone rather than eating the document",
			in:   "---\nname: x\n# Title\n",
			want: "---\nname: x\n# Title\n",
		},
		{
			name: "keeps a horizontal rule that appears later in the body",
			in:   "# Title\n\n---\n\nmore\n",
			want: "# Title\n\n---\n\nmore\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripFrontmatter(tt.in); got != tt.want {
				t.Errorf("stripFrontmatter() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The prompt is only useful if the embedded loop actually describes the loop.
// These are the load-bearing instructions; losing one silently would ship a
// prompt that reads fine and verifies nothing.
func TestEmbeddedLoopCoversTheContract(t *testing.T) {
	text, _ := NewVerifyPrompt().Get(nil)
	body := text.Messages[0].Content.Text

	required := []string{
		"get_orgs",         // preflight probe
		"get_environments", // resolution
		"commit_hash",      // the commit match
		"ready",            // the serving signal
		"bypass_token",     // authenticated access
		"stopped",          // stopped/retired environments never become ready
		"processing",       // build in flight
	}

	for _, needle := range required {
		if !strings.Contains(body, needle) {
			t.Errorf("embedded loop is missing %q, which the loop depends on", needle)
		}
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// An acceptance check is optional, so the loop has to tell the agent what to do
// when there is none: report the narrower result rather than stop and ask.
func TestEmbeddedLoopMakesTheAcceptanceCheckOptional(t *testing.T) {
	text, _ := NewVerifyPrompt().Get(nil)
	body := text.Messages[0].Content.Text

	required := []string{
		"The `acceptance` argument", // where a caller-supplied check comes from
		"Serving your commit",       // the report form when no check ran
		"none configured",           // and what it says about the check
		"do not stop",               // the old behaviour, now explicitly ruled out
		"SHIPYARD_URL=",             // the variables a documented check can rely on
		"SHIPYARD_TOKEN=",
		"cannot run at all", // a broken check is a failure, not a missing one
	}

	for _, needle := range required {
		if !strings.Contains(body, needle) {
			t.Errorf("embedded loop is missing %q, which the optional-check path depends on", needle)
		}
	}

	// The distinction the whole prompt exists to protect. Checked by the
	// sentences that carry it, not by words that appear everywhere.
	for _, rule := range []string{
		"reporting anything\nless than Verified as verified is worse than reporting nothing",
		"Never report Verified from it",
	} {
		if !strings.Contains(body, rule) {
			t.Errorf("embedded loop is missing the rule %q", rule)
		}
	}
}

// Verified has to mean the change itself was exercised, not just that an
// existing suite still passes.
func TestEmbeddedLoopRequiresCoverage(t *testing.T) {
	text, _ := NewVerifyPrompt().Get(nil)
	body := text.Messages[0].Content.Text

	required := map[string]string{
		"Coverage:":                         "every report says what the checks cover",
		"Passed, not covered":               "a passing run with gaps is its own result",
		"Observed":                          "evidence that cannot be rerun is its own result",
		"agent-written, review it":          "added checks are marked for review",
		"`curl`":                            "checks are not limited to e2e tests",
		"A route `url` does not expose":     "internal routes are checked from inside the service",
		"Checks run:":                       "commands that are not committed tests are reported verbatim",
		"Reproducible or it does not count": "an added check must be rerunnable",
		"it must fail":                      "an added check must fail on the base environment",
		"never restart it,":                 "the base environment belongs to the team",
		"`base: not checked (writes data)`": "nothing that writes runs against base",
		"merge-base --is-ancestor":          "the base commit must predate the change",
		"Exactly one of the environments that come back is `ready`": "a stopped or detached base environment does not block the check",
		"$(git merge-base origin/BASE PUSHED_SHA)":                  "the base commit must predate the branch point, not just the head",
		"the test has to be run again there":                        "a pre-push result does not count for a committed test",
		"so do not use it":                                          "the token never goes on the URL, even for access",
		"at the assertion on the changed behavior":                  "a setup failure on base proves nothing",
		"`base: not needed\n(new behavior)`":                        "new behavior needs no base run",
		"**and quote the assertion**":                               "coverage names the assertion, not just a test",
		"Assert on the behavior itself":                             "status-only checks do not count",
		`curl -b "shipyard_token=$SHIPYARD_TOKEN"`:                  "the token travels as a cookie, never on the URL",
		"remove the token's value":                                  "reported output is scrubbed",
		"set `PUSHED_SHA` to the new\n  `HEAD`":                     "a committed test must be pushed and served before it counts",
		"Changed because **you** pushed":                            "the agent's own push is not someone else's build",
		"Never edit or weaken an existing test":                     "passing by weakening a test is ruled out",
		"Verification: read-only":                                   "the repository-level switch",
		"verify a different branch or repository":                   "the user can name another branch or repo in plain words",
		"`git rev-parse origin/<branch>`":                           "a branch that is not checked out is pinned to its pushed commit",
		"**With a description:**":                                   "a plain description of the expected behavior is a valid acceptance check",
		"by what its check will assert, not by the route":           "new vs changed is judged by the assertion, so runs agree",
		"The description decides pass or fail":                      "the check asserts what the user said, not something easier",
		"Do\nthis even when adding checks is off":                   "a check the user asked for runs in read-only mode too",
		"Always quote the description in the report":                "the user sees how their words were read",
	}

	for needle, why := range required {
		if !strings.Contains(body, needle) {
			t.Errorf("embedded loop is missing %q: %s", needle, why)
		}
	}
}

// Editing the running container is fast and unverifiable; the loop has to fence
// it in so it can never produce the verdict.
func TestEmbeddedLoopGuardsLiveEdits(t *testing.T) {
	text, _ := NewVerifyPrompt().Get(nil)
	body := text.Messages[0].Content.Text

	required := map[string]string{
		"sha256sum":                           "the mapping probe and every write are checked by hash",
		"provisional":                         "results from an edited container are not the verdict",
		"push once":                           "the final result comes from a clean rebuild",
		"restore every file":                  "abandoned edits are undone",
		"delete every file you created":       "files the agent added are removed too",
		"multiple of 4 characters":            "base64 chunks decode on their own",
		"then `mv` it over the target":        "the watcher never sees a half-written file",
		"cat /proc/*/cmdline":                 "the watcher is found among all processes",
		"rerun the mapping probe":             "the rebuild is confirmed to have replaced edited files",
		"If its description says DISABLED":    "no live edits when exec is off",
		"**Never** edit the base environment": "live edits stay in this change's environment",
	}

	for needle, why := range required {
		if !strings.Contains(body, needle) {
			t.Errorf("embedded loop is missing %q: %s", needle, why)
		}
	}
}

// A new environment's first build reports a null commit_hash while processing
// is true, for minutes. If the null rule comes first, an agent verifying a
// freshly opened pull request reports its environment as stopped.
func TestPollingContractChecksProcessingBeforeNullCommit(t *testing.T) {
	text, _ := NewVerifyPrompt().Get(nil)
	body := text.Messages[0].Content.Text

	processing := strings.Index(body, "processing == true")
	null := strings.Index(body, "commit_hash is null, not processing")
	if processing == -1 || null == -1 {
		t.Fatalf("polling contract is missing a rule: processing at %d, null commit at %d", processing, null)
	}
	if processing > null {
		t.Error("the polling contract must check processing before treating a null commit_hash as stopped")
	}
}
