//go:build !windows

package app

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// killTree kills git and every process under it: a killed git leaves its ssh
// or credential helper running otherwise. It does not move git to its own
// process group, which would stop ssh reading a passphrase from the terminal.
func killTree(pid int) error {
	// Listed before the kill: a killed process's children are reparented
	tree := descendants(pid, 0)
	err := syscall.Kill(pid, syscall.SIGKILL)
	for _, child := range tree {
		_ = syscall.Kill(child, syscall.SIGKILL)
	}
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

// descendants lists the processes under pid through pgrep, or none where
// pgrep is missing.
func descendants(pid, depth int) []int {
	if depth > 8 {
		return nil
	}
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		child, err := strconv.Atoi(field)
		if err != nil {
			continue
		}
		pids = append(pids, child)
		pids = append(pids, descendants(child, depth+1)...)
	}
	return pids
}
