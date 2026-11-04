//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A git that stalls leaves its ssh or credential helper running when only git
// is killed: the timeout must stop the helper too.
func TestRunGitTimeoutKillsHelpers(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "helper.pid")
	// The fake git starts a helper that outlives it, as ssh would, then waits
	script := "#!/bin/sh\nsleep 300 &\necho $! > " + pidFile + "\nwait\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	defer func(old time.Duration) { gitTimeout = old }(gitTimeout)
	gitTimeout = 500 * time.Millisecond

	_, err := runGit("ls-remote", "origin")
	if err == nil || !strings.Contains(err.Error(), "no answer within") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	// The helper is reparented when killed, so poll until it is gone
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("helper %d still runs after the timeout", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
