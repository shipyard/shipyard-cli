//go:build !windows

package commands

import (
	"fmt"
	"os"
	"syscall"

	"github.com/shipyard/shipyard-cli/pkg/selfupdate"
)

// reexec replaces this process with the upgraded binary, running the same
// command.
func reexec(in *selfupdate.Installer) {
	env := append(os.Environ(), reexecEnv+"=1")
	if err := syscall.Exec(newBinaryPath(in), os.Args, env); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Upgraded, but could not restart: %v\nRun your command again.\n", err)
		os.Exit(1)
	}
}
