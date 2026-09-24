package prompts

import (
	_ "embed"
	"fmt"
	"strings"
)

// verifyLoop is the Shipyard verification loop an agent follows to check its own
// change against the preview environment for its branch.
//
// It is embedded rather than inlined so the prompt text and the document we hand
// to customers cannot drift apart: this file IS that document.
//
//go:embed verify_loop.md
var verifyLoop string

// VerifyPrompt exposes the verification loop as a user-invocable prompt.
type VerifyPrompt struct{}

// NewVerifyPrompt creates the verification loop prompt.
func NewVerifyPrompt() *VerifyPrompt {
	return &VerifyPrompt{}
}

func (p *VerifyPrompt) Definition() PromptDefinition {
	return PromptDefinition{
		Name: "shipyard_verify",
		Description: "Verify a pushed change against the Shipyard preview environment for its branch: " +
			"find the environment, wait until it serves that exact commit, reach it with the bypass " +
			"token, run the acceptance check if there is one, check that the change itself is covered, " +
			"and report the result.",
		Arguments: []PromptArgument{
			// The only argument. The branch, the repository and a read-only run
			// all come from the working directory, the repository's docs, or what
			// the user says in the conversation. More arguments would only add
			// positions to fill for clients that pass arguments by position.
			{
				Name: "acceptance",
				Description: "What decides pass or fail: a command to run against the environment URL " +
					"('npm run test:e2e'), or a plain description of the expected behavior ('the header " +
					"is blue', 'GET /api/widgets returns count as a number') that the assistant turns into " +
					"a check. Defaults to whatever the repository documents.",
				Required: false,
			},
		},
	}
}

func (p *VerifyPrompt) Get(args map[string]string) (GetResult, error) {
	// The embedded file doubles as the document we hand to customers, so it
	// carries YAML frontmatter. That is noise in a prompt body: the name and
	// description are already in the definition.
	text := stripFrontmatter(verifyLoop)

	// Arguments are optional. When the caller supplies one, append it, so the
	// agent acts on this run's choice rather than the repository's default.
	if known := knownTarget(args); known != "" {
		text = strings.TrimRight(text, "\n") + "\n\n## This invocation\n\n" + known + "\n"
	}

	return GetResult{
		Description: "Shipyard verification loop",
		Messages:    textMessage(text),
	}, nil
}

// stripFrontmatter removes a leading YAML frontmatter block, if present. A file
// without one is returned unchanged, as is one whose block is never closed.
func stripFrontmatter(doc string) string {
	const fence = "---"

	if !strings.HasPrefix(doc, fence+"\n") {
		return doc
	}

	rest := doc[len(fence)+1:]
	end := strings.Index(rest, "\n"+fence+"\n")
	if end == -1 {
		return doc
	}

	return strings.TrimLeft(rest[end+len(fence)+2:], "\n")
}

// knownTarget renders the acceptance argument, if the caller supplied one.
func knownTarget(args map[string]string) string {
	// Claude Code splits prompt arguments on every space, quotes or not, so a
	// quoted multi-word check arrives as its first word with an opening quote
	// and nothing closing it; the rest of the words are dropped. The agent can
	// still see what the user typed, so point it there.
	if cutAtSpace(args["acceptance"]) {
		return "The `acceptance` argument arrived cut off at its first space: this client splits " +
			"prompt arguments on spaces, even inside quotes. Take the acceptance check from the full " +
			"text the user typed after the command, between the quotes, and use it in Step 5a in place " +
			"of anything the repository documents."
	}

	acceptance := argValue(args["acceptance"])
	if acceptance == "" {
		return ""
	}

	// What is passed here outranks whatever the repository documents: the
	// caller is looking at this run, the documentation was written for the
	// general case. Whether it is a command or a description is the agent's
	// call (Step 5a): only it can see what is on its PATH and in the repo.
	fence := codeFence(acceptance)

	return fmt.Sprintf("Use this as the acceptance check in Step 5a, in place of anything the repository "+
		"documents. Run it if it is a command; if it describes the expected behavior, write a check that "+
		"asserts exactly that:\n\n%s\n%s\n%s", fence, acceptance, fence)
}

// cutAtSpace reports whether an argument opens with a quote that never closes:
// the first word of a quoted phrase a client split on spaces.
func cutAtSpace(raw string) bool {
	v := strings.TrimSpace(raw)
	if v == "" || (v[0] != '"' && v[0] != '\'') {
		return false
	}

	return len(v) == 1 || v[len(v)-1] != v[0]
}

// argValue trims an argument and removes one layer of matching quotes. Clients
// that pass prompt arguments by position hand quotes through literally: in
// Claude Code, `"make"` arrives with the quotes, and `""` as two characters
// the agent would otherwise try to run.
func argValue(raw string) string {
	v := strings.TrimSpace(raw)
	if len(v) >= 2 {
		if first, last := v[0], v[len(v)-1]; first == last && (first == '"' || first == '\'') {
			v = strings.TrimSpace(v[1 : len(v)-1])
		}
	}

	return v
}

// codeFence returns a backtick fence longer than any backtick run in s, so a
// command that contains ``` cannot close the block early and spill into the
// prompt as instructions.
func codeFence(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}

	return strings.Repeat("`", max(3, longest+1))
}
