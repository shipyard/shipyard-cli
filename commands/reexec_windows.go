//go:build windows

package commands

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/shipyard/shipyard-cli/pkg/selfupdate"
)

// reexec runs the same command on the upgraded binary and exits with its
// status. Windows can't replace a running process, so it's a child instead.
func reexec(in *selfupdate.Installer) {
	cmd := exec.Command(newBinaryPath(in), os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), reexecEnv+"=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		os.Exit(0)
	case errors.As(err, &exitErr):
		os.Exit(exitErr.ExitCode())
	default:
		_, _ = fmt.Fprintf(os.Stderr, "Upgraded, but could not restart: %v\nRun your command again.\n", err)
		os.Exit(1)
	}
}
