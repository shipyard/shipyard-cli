package commands

import (
	"os"
	"os/exec"
	"strings"

	"github.com/shipyard/shipyard-cli/pkg/selfupdate"
)

// newBinaryPath finds the upgraded binary. It looks it up the way the shell
// did, because a Homebrew install's own Cellar directory may already be gone,
// and falls back to the replaced file for a direct install.
func newBinaryPath(in *selfupdate.Installer) string {
	path := os.Args[0]
	if strings.ContainsAny(path, `/\`) {
		return path
	}
	if p, err := exec.LookPath(path); err == nil {
		return p
	}
	return in.Exe
}
