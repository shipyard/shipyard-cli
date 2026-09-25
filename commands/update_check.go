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
	// noUpdateCheckEnv turns the update check off.
	noUpdateCheckEnv = "SHIPYARD_NO_UPDATE_CHECK"
	// noticeWait is how long a finished command waits for the background
	// check. A slower check prints nothing this time and is retried next run.
	noticeWait = time.Second
)

// pendingNotice receives the background check's result; nil when no check runs.
var pendingNotice chan *selfupdate.Notice

// startUpdateCheck starts checking for a new release in the background while
// cmd runs. showUpdateNotice prints the result once the command is done.
func startUpdateCheck(cmd *cobra.Command) {
	selfupdate.CleanupOld()
	if !shouldCheckForUpdate(cmd) {
		return
	}
	home := homedir.HomeDir()
	if home == "" {
		return
	}
	n := &selfupdate.Notifier{
		Current:   version.Version,
		StatePath: selfupdate.StatePath(home),
		Client:    selfupdate.NewClient(15 * time.Second),
		Now:       time.Now,
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	pendingNotice = make(chan *selfupdate.Notice, 1)
	go func() { pendingNotice <- n.Check(ctx) }()
}

// showUpdateNotice prints what the background check found, on stderr, after
// the command's own output.
func showUpdateNotice() {
	if pendingNotice == nil {
		return
	}
	select {
	case notice := <-pendingNotice:
		if notice.Text == "" {
			return
		}
		_, _ = os.Stderr.WriteString("\n" + notice.Text)
		notice.Shown()
	case <-time.After(noticeWait):
	}
}

func shouldCheckForUpdate(cmd *cobra.Command) bool {
	if !selfupdate.IsRelease(version.Version) {
		return false
	}
	if os.Getenv(noUpdateCheckEnv) != "" || os.Getenv("CI") != "" || !viper.GetBool("update_check") {
		return false
	}
	if underAgent() {
		return false
	}
	// The notice goes to stderr; only print it where a person will see it.
	if !isTerminal(os.Stderr) {
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

// agentEnvVars are set by AI coding agents in the shells they run commands
// in. The notice never blocks, but it's noise in output an agent parses.
var agentEnvVars = []string{
	"CLAUDECODE",                     // Claude Code
	"GEMINI_CLI",                     // Gemini CLI
	"CURSOR_AGENT",                   // Cursor's agent terminal
	"CODEX_SANDBOX",                  // Codex CLI
	"CODEX_SANDBOX_NETWORK_DISABLED", // Codex CLI
}

func underAgent() bool {
	for _, v := range agentEnvVars {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
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
