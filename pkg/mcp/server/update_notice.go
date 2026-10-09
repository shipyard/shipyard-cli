package server

import (
	"fmt"
	"os"

	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/selfupdate"
)

// cachedLatestVersion is the newest release the CLI's own update check last
// saw, read from ~/.shipyard/update-state.json. It never goes to the network:
// a client is waiting on initialize. The check itself only runs from a
// terminal, so the value is as fresh as the user's last interactive command.
// The CLI's opt-outs (SHIPYARD_NO_UPDATE_CHECK, update_check: false) apply.
func cachedLatestVersion() string {
	if os.Getenv("SHIPYARD_NO_UPDATE_CHECK") != "" || !viper.GetBool("update_check") {
		return ""
	}
	home := selfupdate.HomeDir()
	if home == "" {
		return ""
	}
	return selfupdate.LoadState(selfupdate.StatePath(home)).LatestVersion
}

// staleNotice is appended to the server instructions when a newer release is
// known. The CLI's update notice goes to stderr after a command finishes, which
// `mcp serve` never does and clients don't show, so the agent is the only one
// who can pass it on.
func staleNotice(current, latest string) string {
	if !selfupdate.IsRelease(current) || !selfupdate.IsNewer(current, latest) {
		return ""
	}
	return fmt.Sprintf("Shipyard CLI %s is out of date (latest %s): if a tool is missing or fails, "+
		"suggest `shipyard upgrade`.", current, latest)
}
