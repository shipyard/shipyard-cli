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
			"token, run the acceptance check if there is one, and report the result.",
		Arguments: []PromptArgument{
			// First, because clients that pass prompt arguments by position would
			// otherwise make callers spell out branch and repo just to reach it.
			{
				Name: "acceptance_command",
				Description: "Command that decides pass or fail, run against the environment URL. " +
					"Defaults to whatever the repository documents. Omit it and document nothing to " +
					"confirm the environment is serving the commit without running any check.",
				Required: false,
			},
			{
				Name:        "branch",
				Description: "Branch to verify. Defaults to the current branch when omitted.",
				Required:    false,
			},
			{
				Name:        "repo_name",
				Description: "Repository as Shipyard knows it. Defaults to the current repository when omitted.",
				Required:    false,
			},
		},
	}
}

func (p *VerifyPrompt) Get(args map[string]string) (GetResult, error) {
	// The embedded file doubles as the document we hand to customers, so it
	// carries YAML frontmatter. That is noise in a prompt body: the name and
	// description are already in the definition.
	text := stripFrontmatter(verifyLoop)

	// Arguments are optional: the loop tells the agent to read branch and repo
	// from the working directory. When the caller supplies them, say so, so the
	// agent does not re-derive them and quietly verify a different branch.
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

// knownTarget renders whichever of branch/repo_name/acceptance_command the
// caller supplied.
func knownTarget(args map[string]string) string {
	branch := strings.TrimSpace(args["branch"])
	repo := strings.TrimSpace(args["repo_name"])
	command := strings.TrimSpace(args["acceptance_command"])

	var lines []string

	switch {
	case branch != "" && repo != "":
		lines = append(lines, fmt.Sprintf("Verify branch `%s` of repository `%s`. Use these instead of "+
			"reading them from the working directory.", branch, repo))
	case branch != "":
		lines = append(lines, fmt.Sprintf("Verify branch `%s`. Read the repository name from the working directory.", branch))
	case repo != "":
		lines = append(lines, fmt.Sprintf("Verify repository `%s`. Read the branch from the working directory.", repo))
	}

	// A command passed here outranks whatever the repository documents: the
	// caller is looking at this run, the documentation was written for the
	// general case.
	if command != "" {
		fence := codeFence(command)
		lines = append(lines, fmt.Sprintf("Run this as the acceptance check in Step 5, in place of anything "+
			"the repository documents:\n\n%s\n%s\n%s", fence, command, fence))
	}

	return strings.Join(lines, "\n\n")
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
