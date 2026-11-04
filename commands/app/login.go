package app

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// githubLoginRE matches a GitHub login: letters, digits and single inner
// dashes, at most 39 characters.
var githubLoginRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9]|-[A-Za-z0-9]){0,38}$`)

// noreplyEmailRE matches GitHub's noreply commit email, "ID+login@" or the
// older "login@users.noreply.github.com".
var noreplyEmailRE = regexp.MustCompile(`(?i)^(?:\d+\+)?([a-z0-9-]+)@users\.noreply\.github\.com$`)

// ghTimeout bounds the GitHub CLI lookup, which calls GitHub's API.
var ghTimeout = 10 * time.Second

// runGhLogin is the login the GitHub CLI is signed in as, or "" when gh is
// missing, signed out or slow.
func runGhLogin() string {
	path, err := exec.LookPath("gh")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "api", "user", "--jq", ".login")
	cmd.Cancel = func() error { return killTree(cmd.Process.Pid) }
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// localGitHubLogin guesses the GitHub login of whoever runs add: the GitHub
// CLI's sign-in, the login in a noreply commit email, then git's github.user.
// git's user.name is a display name and other emails don't say the login, so
// neither is used. It is only offered as a default to confirm: a wrong guess
// would let someone else into the environment.
func localGitHubLogin(git gitRunner, gh func() string) string {
	sources := []func() string{
		func() string {
			if gh == nil {
				return ""
			}
			return gh()
		},
		func() string {
			email, _ := git("config", "user.email")
			if m := noreplyEmailRE.FindStringSubmatch(strings.TrimSpace(email)); m != nil {
				return m[1]
			}
			return ""
		},
		func() string {
			login, _ := git("config", "github.user")
			return login
		},
	}
	for _, source := range sources {
		if login := strings.TrimSpace(source()); githubLoginRE.MatchString(login) {
			return login
		}
	}
	return ""
}
