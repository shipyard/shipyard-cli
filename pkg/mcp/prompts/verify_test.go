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

	if len(def.Arguments) != 3 {
		t.Fatalf("expected 3 arguments, got %d", len(def.Arguments))
	}

	for _, arg := range def.Arguments {
		if arg.Required {
			t.Errorf("argument %s should be optional: the loop reads branch and repo from the working "+
				"directory, and runs without an acceptance command when there is none", arg.Name)
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
			name:         "branch and repo are both named",
			args:         map[string]string{"branch": "feat/x", "repo_name": "shipyard"},
			wantContains: []string{"## This invocation", "`feat/x`", "`shipyard`"},
		},
		{
			name:         "branch only",
			args:         map[string]string{"branch": "feat/x"},
			wantContains: []string{"## This invocation", "`feat/x`", "Read the repository name"},
		},
		{
			name:         "repo only",
			args:         map[string]string{"repo_name": "shipyard"},
			wantContains: []string{"## This invocation", "`shipyard`", "Read the branch"},
		},
		{
			name:         "whitespace-only arguments are ignored",
			args:         map[string]string{"branch": "   ", "repo_name": "", "acceptance_command": "  "},
			wantContains: []string{"# Shipyard Verification Loop"},
			wantMissing:  []string{"## This invocation"},
		},
		{
			name:         "acceptance command alone is enough to render the invocation",
			args:         map[string]string{"acceptance_command": "npm run test:e2e"},
			wantContains: []string{"## This invocation", "npm run test:e2e", "in place of anything"},
			wantMissing:  []string{"Verify branch"},
		},
		{
			name: "acceptance command rides along with the target",
			args: map[string]string{
				"branch":             "feat/x",
				"repo_name":          "shipyard",
				"acceptance_command": "make test.e2e",
			},
			wantContains: []string{"`feat/x`", "`shipyard`", "make test.e2e"},
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
		"acceptance_command",  // where a caller-supplied command comes from
		"Serving your commit", // the report form when no check ran
		"none configured",     // and what it says about the check
		"do not stop",         // the old behaviour, now explicitly ruled out
	}

	for _, needle := range required {
		if !strings.Contains(body, needle) {
			t.Errorf("embedded loop is missing %q, which the optional-check path depends on", needle)
		}
	}

	// The distinction the whole prompt exists to protect.
	if !strings.Contains(body, "Never call") || !strings.Contains(body, "verified") {
		t.Error("embedded loop must forbid calling an unchecked environment verified")
	}
}
