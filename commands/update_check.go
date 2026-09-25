package commands

import (
	"context"
	"os"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/util/homedir"

	"github.com/shipyard/shipyard-cli/pkg/selfupdate"
	"github.com/shipyard/shipyard-cli/version"
)

const (
	// noUpdateCheckEnv turns the startup check off.
	noUpdateCheckEnv = "SHIPYARD_NO_UPDATE_CHECK"
	// reexecEnv marks the process the CLI starts after upgrading, so the new
	// binary doesn't check again. It's cleared on arrival: a shell that
	// command starts (exec, telepresence) must not inherit it.
	reexecEnv = "SHIPYARD_UPGRADE_REEXEC"
)

// checkForUpdate runs the startup update check before cmd, and re-runs the
// command on the new binary if the user chose to upgrade.
func checkForUpdate(cmd *cobra.Command) {
	selfupdate.CleanupOld()
	if os.Getenv(reexecEnv) != "" {
		_ = os.Unsetenv(reexecEnv)
		return
	}
	if !shouldCheckForUpdate(cmd) {
		return
	}
	home := homedir.HomeDir()
	if home == "" {
		return
	}
	installer, err := selfupdate.NewInstaller(os.Stderr, version.Version)
	if err != nil {
		return
	}
	s := &selfupdate.Startup{
		Current:   version.Version,
		StatePath: selfupdate.StatePath(home),
		Client:    selfupdate.NewClient(15 * time.Second),
		In:        os.Stdin,
		Out:       os.Stderr,
		Now:       time.Now,
		Install:   installer.Install,
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if s.Run(ctx) {
		reexec(installer)
	}
}

func shouldCheckForUpdate(cmd *cobra.Command) bool {
	if !selfupdate.IsRelease(version.Version) {
		return false
	}
	if os.Getenv(noUpdateCheckEnv) != "" || os.Getenv("CI") != "" || !viper.GetBool("update_check") {
		return false
	}
	// Only prompt a person at a terminal: stdin to answer, stderr to see the
	// prompt, and stdout so `shipyard ... | jq` and redirects never block. A
	// script started from a terminal inherits all three and can't be told
	// apart; SHIPYARD_NO_UPDATE_CHECK covers that.
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) || !isTerminal(os.Stderr) {
		return false
	}
	switch topLevelName(cmd) {
	case "mcp", "upgrade", "update", "completion", "help", "__complete", "__completeNoDesc":
		// mcp serve speaks JSON-RPC to a client, not a person; upgrade does
		// its own check; completion runs on every Tab.
		return false
	}
	return true
}

// topLevelName is the name of the command directly under root that cmd
// belongs to: "get" for `shipyard get environments`.
func topLevelName(cmd *cobra.Command) string {
	for cmd.HasParent() && cmd.Parent().HasParent() {
		cmd = cmd.Parent()
	}
	return cmd.Name()
}

func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}
