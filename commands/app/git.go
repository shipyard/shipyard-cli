package app

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// gitRunner runs git with args in the working directory and returns its
// trimmed stdout.
type gitRunner func(args ...string) (string, error)

// gitTimeout bounds each git command: ls-remote talks to the remote, and a
// stalled SSH connection or credential helper would otherwise hang add. It
// leaves time to type an SSH passphrase.
var gitTimeout = 2 * time.Minute

func runGit(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Cancel = func() error { return killTree(cmd.Process.Pid) }
	// A helper that escaped the kill can still hold the output pipes: stop
	// waiting for them shortly after the deadline
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("git %s: no answer within %s", strings.Join(args, " "), gitTimeout)
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), withoutCredentials(strings.TrimSpace(string(exitErr.Stderr))))
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

// remoteUserinfoRE matches the user:token@ part of a scheme://user:token@host URL,
// anywhere in the text: git's stderr quotes the URL inside a sentence.
var remoteUserinfoRE = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/@\s'"]*@`)

// withoutCredentials drops the user:token part of a remote URL, so printing it
// cannot leak a token. It does not parse the URL, so one that url.Parse would
// reject (a bad port) loses its token too. scp-style remotes (git@host:path)
// carry none.
func withoutCredentials(remote string) string {
	return remoteUserinfoRE.ReplaceAllString(remote, "$1")
}

// shellSafeRE matches a word a shell reads as itself.
var shellSafeRE = regexp.MustCompile(`^[A-Za-z0-9._/@%+=:,-]+$`)

// shellQuote quotes s for a command the user may paste: git allows ; and $ in
// branch names.
func shellQuote(s string) string {
	if shellSafeRE.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// preflight checks that what Shipyard will build from GitHub is exactly the
// local checkout: origin is on GitHub, the branch is pushed, local HEAD is the
// remote head, and nothing is left uncommitted.
func preflight(git gitRunner) (checkout, error) {
	remote, err := git("remote", "get-url", "origin")
	if err != nil {
		// git's reason: no remote, not a repository, a safe.directory refusal
		return checkout{}, fmt.Errorf("reading origin: %s; run shipyard add from a clone of a GitHub repo", withoutCredentials(err.Error()))
	}
	repo, ok := parseGitHubRemote(remote)
	if !ok {
		return checkout{}, fmt.Errorf("origin (%s) is not a GitHub repo: shipyard add supports GitHub repos only", withoutCredentials(remote))
	}

	// symbolic-ref, not rev-parse --abbrev-ref, which answers heads/<branch>
	// when a tag has the branch's name. It fails when HEAD is detached.
	ref, err := git("symbolic-ref", "-q", "HEAD")
	branch, isBranch := strings.CutPrefix(ref, "refs/heads/")
	if err != nil || !isBranch || branch == "" {
		return checkout{}, fmt.Errorf("HEAD is detached: check out the branch to build")
	}

	heads, err := git("ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil {
		return checkout{}, err
	}
	// ls-remote matches the pattern as a suffix, so refs/heads/x/refs/heads/<branch>
	// matches too: only the exact ref counts
	var remoteSHA string
	for _, line := range strings.Split(heads, "\n") {
		if sha, ref, ok := strings.Cut(line, "\t"); ok && ref == "refs/heads/"+branch {
			remoteSHA = sha
			break
		}
	}
	if remoteSHA == "" {
		return checkout{}, fmt.Errorf("branch %s is not on origin: push it first (git push -u origin %s)", branch, shellQuote(branch))
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
	// git status skips files marked skip-worktree (S) or assume-unchanged
	// (a lowercase tag), so edits to them would go unseen
	files, err := git("ls-files", "-v")
	if err != nil {
		return checkout{}, err
	}
	var hidden []string
	for _, line := range strings.Split(files, "\n") {
		tag, file, ok := strings.Cut(line, " ")
		if ok && (tag == "S" || (tag != "" && unicode.IsLower(rune(tag[0])))) {
			hidden = append(hidden, file)
		}
	}
	if len(hidden) > 0 {
		return checkout{}, fmt.Errorf("git status does not check these files, marked skip-worktree or assume-unchanged, "+
			"so local edits to them could differ from what Shipyard builds: clear the flag (git update-index "+
			"--no-skip-worktree --no-assume-unchanged <file>) or confirm they match origin\n%s", strings.Join(hidden, "\n"))
	}

	return checkout{Repo: repo, Branch: branch, HeadSHA: localSHA}, nil
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
