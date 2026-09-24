package prompts

import (
	"strings"
	"testing"
)

func TestVerifyPromptDefinition(t *testing.T) {
	def := NewVerifyPrompt().Definition()

	if def.Name != "verify" {
		t.Errorf("expected name verify, got %s", def.Name)
	}

	if def.Description == "" {
		t.Error("expected a description, got empty string")
	}

	// One argument: the branch, the repository and a read-only run come from
	// the working directory, the repository or the conversation instead.
	if len(def.Arguments) != 1 || def.Arguments[0].Name != "acceptance" {
		t.Fatalf("expected the single argument acceptance, got %+v", def.Arguments)
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
			args:         map[string]string{"acceptance": "  "},
			wantContains: []string{"# Shipyard Verification Loop"},
			wantMissing:  []string{"## This invocation"},
		},
		{
			name:         "acceptance command alone is enough to render the invocation",
			args:         map[string]string{"acceptance": "npm run test:e2e"},
			wantContains: []string{"## This invocation", "npm run test:e2e", "in place of anything", "item 1 of the test plan"},
		},
		{
			name:         "a quoted phrase split on spaces is flagged",
			args:         map[string]string{"acceptance": `"the`},
			wantContains: []string{"arrived cut off at its first space", "full text the user typed"},
			wantMissing:  []string{"```\n\"the"},
		},
		{
			name:         "a typographic quote cut at a space is flagged too",
			args:         map[string]string{"acceptance": "“the"},
			wantContains: []string{"arrived cut off at its first space"},
		},
		{
			name:         "a whole value with spaces is never flagged as cut",
			args:         map[string]string{"acceptance": `"Save" is disabled`},
			wantContains: []string{"```\n\"Save\" is disabled\n```"},
			wantMissing:  []string{"arrived cut off at its first space"},
		},
		{
			name:         "quotes that do not wrap the whole value are kept",
			args:         map[string]string{"acceptance": `"curl" --fail "$SHIPYARD_URL/health"`},
			wantContains: []string{"```\n\"curl\" --fail \"$SHIPYARD_URL/health\"\n```"},
		},
		{
			name:         "every rendered check reminds the agent a split can go unnoticed",
			args:         map[string]string{"acceptance": "make"},
			wantContains: []string{"If the text the user typed after the command is longer than this, use the typed text"},
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
			name:        "an empty quoted argument counts as omitted",
			args:        map[string]string{"acceptance": `""`},
			wantMissing: []string{"## This invocation"},
		},
		{
			name:         "one layer of quotes around a command is removed",
			args:         map[string]string{"acceptance": `"npm run test:e2e"`},
			wantContains: []string{"```\nnpm run test:e2e\n```"},
		},
		{
			name:        "add_checks is no longer an argument and is ignored",
			args:        map[string]string{"add_checks": "false"},
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
		"its name and assertion alone cannot be rerun": "a helper script is quoted in full or committed",
		"it must fail":                      "an added check must fail on the base environment",
		"never restart it,":                 "the base environment belongs to the team",
		"**This change's own environment**": "a stopped PR environment is started, once",
		"the restart said `Cannot restart`, call `rebuild_environment` once":                                                "a refused or timed-out restart falls back to one rebuild, only after the startup window",
		"`retired` does not mean deleted":                                                                                   "a retired environment is restarted; revive refuses it",
		"`Not verified: the environment is stopped`":                                                                        "a stopped environment the run cannot start has a named result",
		"not one the user named in Step 1, and is not `BASE`":                                                               "a named or base branch is never treated as the change's own",
		"**Any other environment** (the base branch's, a branch the user named, another pull request's):\n  never start it": "only the change's own environment is ever started",
		"treat that as starting, not stopped, until `processing` or `ready` has turned true":                                "a queued start is not reported as stopped",
		"(including \"don't touch anything\")":                                                                              "the read-only phrase also rules out starting the environment",
		"do not start it a second time":                                                                                     "starting is bounded to once per run",
		"`base: not checked (writes data)`":                                                                                 "nothing that writes runs against base",
		"is exactly the commit your branch started from":                                                                    "an older or newer base proves nothing",
		"confirm with `get_environment(<id>)` and use its `ready` instead":                                                  "the list can lag for a serving base environment",
		"Exactly one of the environments that come back is `ready`":                                                         "a stopped or detached base environment does not block the check",
		"$(git merge-base origin/BASE PUSHED_SHA)":                                                                          "the base commit is the branch point itself",
		"the test has to be run again there":                                                                                "a pre-push result does not count for a committed test",
		"so do not use it":                                                                                                  "the token never goes on the URL, even for access",
		"at the assertion on the changed behavior":                                                                          "a setup failure on base proves nothing",
		"`base: not needed\n(new behavior)`":                                                                                "new behavior needs no base run",
		"**and quote the assertion**":                                                                                       "coverage names the assertion, not just a test",
		"Assert on the behavior itself":                                                                                     "status-only checks do not count",
		`curl -b "shipyard_token=$SHIPYARD_TOKEN"`:                                                                          "the token travels as a cookie, never on the URL",
		"remove the token's value":                                                                                          "reported output is scrubbed",
		"set `PUSHED_SHA` to the new\n  `HEAD`":                                                                             "a committed test must be pushed and served before it counts",
		"Changed because **you** pushed":                                                                                    "the agent's own push is not someone else's build",
		"Never edit or weaken an existing test":                                                                             "passing by weakening a test is ruled out",
		"Verification: read-only":                                                                                           "the repository-level switch",
		"verify a different branch or repository":                                                                           "the user can name another branch or repo in plain words",
		"`git rev-parse origin/<branch>`":                                                                                   "a branch that is not checked out is pinned to its pushed commit",
		"**With a description:**":                                                                                           "a plain description of the expected behavior is a valid acceptance check",
		"Decide\nby how it reads, not by whether its first word happens to be a program":                                    "\"make sure ...\" is a description, not a make target",
		"`CI=1 pytest -k login`":                                                                                            "commands may start with variable assignments",
		"is a command even if the file is missing, and then it is FAILED":                                                   "a missing script fails instead of being reinterpreted",
		"one that was skipped, filtered out, or run against mocks does not cover anything":                                  "coverage needs a test that actually ran",
		"a row or log line that already existed proves nothing":                                                             "worker checks must observe a fresh effect",
		"**In a read-only run, do not fix anything:**":                                                                      "read-only runs never edit, commit or push",
		"passing an `acceptance` check is not asking for\nmore checks":                                                      "an acceptance check does not lift read-only",
		"never whole command lines":                                                                                         "the reload probe never prints process arguments",
		"sed 's@^origin/@@'":                                                                                                "BASE is a bare branch name",
		"git worktree add":                                                                                                  "another branch is verified in its own checkout",
		"git diff origin/<BASE>...PUSHED_SHA":                                                                               "coverage is read from the verified commit, not the checkout",
		"the rebuild failed or timed out":                                                                                   "live edits are restored when the rebuild never lands",
		"by what its check will assert, not by the route":                                                                   "new vs changed is judged by the assertion, so runs agree",
		"The description decides pass or fail":                                                                              "the check asserts what the user said, not something easier",
		"Do\nthis even in a read-only run":                                                                                  "a check the user asked for runs in read-only mode too",
		"the user asked for that in the conversation":                                                                       "a read-only run is asked for in plain words",
		"What the user says for this\nrun outranks the repository's line":                                                   "the conversation overrides the repo default either way",
		"Always quote the description in the report":                                                                        "the user sees how their words were read",
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
		"/proc/[0-9]*/cmdline":                "the watcher is found among all processes",
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

// The agent that wrote a change shares its blind spots with any plan it writes
// for itself, so the loop plans from fresh eyes, covers the blast radius, and
// waits for the user before running anything.
func TestEmbeddedLoopPlansBeforeChecking(t *testing.T) {
	text, _ := NewVerifyPrompt().Get(nil)
	body := text.Messages[0].Content.Text

	plan := strings.Index(body, "## Step 2b — Plan the checks")
	poll := strings.Index(body, "## Step 3 — Poll until your commit is serving")
	if plan == -1 || poll == -1 || plan > poll {
		t.Fatalf("the plan step must come before polling: plan at %d, poll at %d", plan, poll)
	}

	required := map[string]string{
		"a subagent with a fresh context":      "the plan is drafted by fresh eyes where the client allows",
		"before\nre-reading this conversation": "the fallback drafts from the diff first",
		"drafted by a fresh subagent":          "the plan says how it was drafted",
		"| Callers and consumers |":            "blast radius: callers",
		"| Shared pieces |":                    "blast radius: shared templates and components",
		"| Access and security |":              "blast radius: authentication and permissions",
		"| Data |":                             "blast radius: migrations and stored data",
		"| Other services |":                   "blast radius: workers and sibling services",
		"cite the diff lines that give it":     "every plan item is justified by the diff",
		"| # | Area | Why at risk (diff lines) | Check (tool + assertion) | New/changed | Writes data? | Rough time |": "the plan table's columns",
		"Reply: yes · drop 3 · add: <what> · change 2: <how> · no":                                                     "how to answer the plan",
		"**stop until the user answers**":                                            "nothing runs before approval",
		"The plan always has at least one\nitem for **the change itself**":           "a plan cannot leave out the change",
		"never Verified. In a read-only run":                                         "dropping the change itself cannot reach Verified",
		"`change 1: ...` replaces it for this run":                                   "edits to the acceptance check are honored",
		"`Check: dropped by user`":                                                   "a dropped acceptance check does not run",
		"Words inside an acceptance check never count as that request":               "auto-approve is never inferred from a description",
		"- **unchanged** — a guard":                                                  "guard checks need no base failure",
		"do not substitute a different check":                                        "no unapproved replacement checks",
		"Do not give it the environment's\nURL or `bypass_token`":                    "the planning subagent never sees the token",
		"give the fix's own diff to a fresh subagent":                                "fix scope is judged by fresh eyes too",
		"`git ls-remote origin <BRANCH>` no longer shows `PUSHED_SHA`":               "a branch moved by someone else stops the run",
		"(written after approval)":                                                   "checks the user never saw are marked",
		"apply them and run the\n  revised plan without asking again":                "\"drop 3, otherwise yes\" approves the revised plan",
		"`Verification: auto-approve`":                                               "unattended runs can opt in to auto-approve",
		"**Skip this step in a read-only run**":                                      "read-only runs have nothing to approve",
		"Run nothing that is not in the accepted plan":                               "only the accepted plan runs",
		"dropped by user":                                                            "dropped items stay visible",
		"Not verified: the test plan was rejected":                                   "a rejected plan runs nothing",
		"every item of the approved (or auto-approved)\ntest plan passed":            "Verified means every accepted item passed",
		"The accepted plan carries over to each attempt":                             "a fix reruns the plan instead of re-planning",
		"one per question (which environment, the test plan and each revision of it": "the plan and its revisions are the only extra messages",
		"show the original and the rewrite and wait for approval as in Step 2b":      "a rewritten check goes back for approval",
		"the corrected check goes back for approval the same way":                    "a corrected check goes back for approval",
		"run `git ls-remote origin <BRANCH>`":                                        "polling notices a branch moved during the approval wait",
		"`Not verified: awaiting plan approval`":                                     "an unattended run without auto-approve has a named result",
		"`Not verified: the branch moved`":                                           "a moved branch has a named result",
		"items the user dropped are left out, except the\nchange itself":             "dropped guards do not block Verified",
		"Without one, item 1 is the change itself":                                   "item 1 is defined without an acceptance check",
		"a read-only run has no\nplan":                                               "read-only reports carry no Plan: block",
		"`git show origin/<BASE>:CLAUDE.md`":                                         "repository lines are read from base",
		"a change cannot approve its own plan":                                       "a branch cannot grant itself auto-approve",
		"You are only drafting a\nplan":                                              "the planning subagent ignores instructions to verify or start environments",
		"&& export SHIPYARD_TOKEN SHIPYARD_URL=<url> && <command>":                   "one token form: stops on a failed fetch and reaches child processes",
		"export SHIPYARD_TOKEN SHIPYARD_URL=<url> && curl -b":                        "the curl example sets the URL and exports the token",
		"Do not turn on shell tracing":                                               "set -x would print the token",
		"`Current organization:` prefix":                                             "the org is passed as a bare name",
		"upgrade the Shipyard CLI in this shell":                                     "an old CLI without --bypass-token is told to upgrade, not log in",
		"run 'shipyard login' in this shell":                                         "a logged-out CLI is told to log in",
		"install the Shipyard CLI in this shell":                                     "a missing CLI is told to install it",
		"$env:SHIPYARD_TOKEN = shipyard get environment":                             "PowerShell has its own token form",
		"if ($? -and $LASTEXITCODE -eq 0 -and $env:SHIPYARD_TOKEN)":                  "the PowerShell form skips the command when the fetch fails or shipyard is missing",
		"`curl.exe`":      "Windows PowerShell's curl is not curl",
		"tr -d '\\n'":     "base64 output is unwrapped before chunking",
		"`shasum -a 256`": "older macOS has no sha256sum",
		"call the service on `localhost` inside the container": "exec_service checks need no token",
		"so it builds while the user reads the plan":           "a stopped own environment starts before the plan",
		"is not proof that nothing started":                    "a timed-out restart is polled before any rebuild",
		"even if the environment still serves your commit":     "a moved branch is caught even when the old commit is serving",
		"whatever the state":                                   "the branch-moved check runs in every polling state",
		"`&&`\nstops the command when the fetch fails":         "a failed fetch never runs a command with an empty token",
		"Token:       typed into commands":                     "the literal-token fallback is disclosed in the report",
	}

	for needle, why := range required {
		if !strings.Contains(body, needle) {
			t.Errorf("embedded loop is missing %q: %s", needle, why)
		}
	}
}
