package app

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// gitRunner runs git with args in the working directory and returns its
// trimmed stdout.
type gitRunner func(args ...string) (string, error)

func runGit(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// checkout is the repo state `shipyard add` creates an app from.
type checkout struct {
	Repo    string // owner/name on GitHub
	Branch  string
	HeadSHA string
}

var githubRemoteRE = regexp.MustCompile(`^(?:https://(?:[^@/]+@)?github\.com/|git@github\.com:|ssh://git@github\.com/)([^/]+)/([^/]+?)(?:\.git)?/?$`)

// parseGitHubRemote returns owner/name for a GitHub remote URL.
func parseGitHubRemote(remote string) (string, bool) {
	m := githubRemoteRE.FindStringSubmatch(strings.TrimSpace(remote))
	if m == nil {
		return "", false
	}
	return m[1] + "/" + m[2], true
}

// remoteUserinfoRE matches the user:token@ part of a scheme://user:token@host URL.
var remoteUserinfoRE = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*://)[^/@]*@`)

// withoutCredentials drops the user:token part of a remote URL, so printing it
// cannot leak a token. It does not parse the URL, so one that url.Parse would
// reject (a bad port) loses its token too. scp-style remotes (git@host:path)
// carry none.
func withoutCredentials(remote string) string {
	return remoteUserinfoRE.ReplaceAllString(remote, "$1")
}

// preflight checks that what Shipyard will build from GitHub is exactly the
// local checkout: origin is on GitHub, the branch is pushed, local HEAD is the
// remote head, and nothing is left uncommitted.
func preflight(git gitRunner) (checkout, error) {
	remote, err := git("remote", "get-url", "origin")
	if err != nil {
		return checkout{}, fmt.Errorf("this directory has no git remote named origin: run shipyard add from a clone of a GitHub repo")
	}
	repo, ok := parseGitHubRemote(remote)
	if !ok {
		return checkout{}, fmt.Errorf("origin (%s) is not a GitHub repo: shipyard add supports GitHub repos only", withoutCredentials(remote))
	}

	branch, err := git("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return checkout{}, err
	}
	if branch == "HEAD" {
		return checkout{}, fmt.Errorf("HEAD is detached: check out the branch to build")
	}

	heads, err := git("ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil {
		return checkout{}, err
	}
	remoteSHA, _, _ := strings.Cut(heads, "\t")
	if remoteSHA == "" {
		return checkout{}, fmt.Errorf("branch %s is not on origin: push it first (git push -u origin %s)", branch, branch)
	}

	localSHA, err := git("rev-parse", "HEAD")
	if err != nil {
		return checkout{}, err
	}
	if localSHA != remoteSHA {
		return checkout{}, fmt.Errorf("local %s (%s) differs from origin (%s): push or pull so Shipyard builds what you have", branch, short(localSHA), short(remoteSHA))
	}

	// Set both explicitly: a status.showUntrackedFiles=no config would hide new
	// files, and a submodule ignore=dirty setting changed submodule code, neither
	// of which Shipyard would build
	status, err := git("status", "--porcelain", "--untracked-files=normal", "--ignore-submodules=none")
	if err != nil {
		return checkout{}, err
	}
	if status != "" {
		return checkout{}, fmt.Errorf("the working tree has uncommitted changes, which Shipyard would not build: commit and push them, or stash them\n%s", status)
	}

	return checkout{Repo: repo, Branch: branch, HeadSHA: localSHA}, nil
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
