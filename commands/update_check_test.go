package commands

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/version"
)

func TestTopLevelName(t *testing.T) {
	root := &cobra.Command{Use: "shipyard"}
	mcp := &cobra.Command{Use: "mcp"}
	serve := &cobra.Command{Use: "serve"}
	upgrade := &cobra.Command{Use: "upgrade", Aliases: []string{"update"}, Run: func(*cobra.Command, []string) {}}
	root.AddCommand(mcp, upgrade)
	mcp.AddCommand(serve)

	if got := topLevelName(serve); got != "mcp" {
		t.Errorf("mcp serve: got %q", got)
	}
	if got := topLevelName(mcp); got != "mcp" {
		t.Errorf("mcp: got %q", got)
	}
	// The alias resolves to the command's own name.
	found, _, err := root.Find([]string{"update"})
	if err != nil || topLevelName(found) != "upgrade" {
		t.Errorf("update alias: got %v, %v", found, err)
	}
}

func TestShouldCheckForUpdate(t *testing.T) {
	root := &cobra.Command{Use: "shipyard"}
	get := &cobra.Command{Use: "get"}
	envs := &cobra.Command{Use: "environments"}
	mcp := &cobra.Command{Use: "mcp"}
	serve := &cobra.Command{Use: "serve"}
	root.AddCommand(get, mcp)
	get.AddCommand(envs)
	mcp.AddCommand(serve)

	setup := func(t *testing.T) {
		for _, v := range append([]string{noUpdateCheckEnv, "CI"}, agentEnvVars...) {
			t.Setenv(v, "")
		}
		oldVersion, oldTerminal := version.Version, stderrIsTerminal
		version.Version = "1.9.0"
		stderrIsTerminal = func() bool { return true }
		viper.Set("update_check", true)
		t.Cleanup(func() {
			version.Version, stderrIsTerminal = oldVersion, oldTerminal
			viper.Set("update_check", true)
		})
	}

	setup(t)
	if !shouldCheckForUpdate(envs) {
		t.Fatal("a release build in a terminal should check")
	}

	for name, change := range map[string]func(t *testing.T){
		"dev build":             func(*testing.T) { version.Version = "undefined" },
		"snapshot":              func(*testing.T) { version.Version = "1.9.1-SNAPSHOT-abc" },
		"opt-out env":           func(t *testing.T) { t.Setenv(noUpdateCheckEnv, "1") },
		"CI":                    func(t *testing.T) { t.Setenv("CI", "true") },
		"config off":            func(*testing.T) { viper.Set("update_check", false) },
		"Claude Code":           func(t *testing.T) { t.Setenv("CLAUDECODE", "1") },
		"Codex":                 func(t *testing.T) { t.Setenv("CODEX_SANDBOX", "seatbelt") },
		"stderr not a terminal": func(*testing.T) { stderrIsTerminal = func() bool { return false } },
	} {
		t.Run(name, func(t *testing.T) {
			setup(t)
			change(t)
			if shouldCheckForUpdate(envs) {
				t.Error("checked anyway")
			}
		})
	}

	t.Run("mcp serve", func(t *testing.T) {
		setup(t)
		if shouldCheckForUpdate(serve) {
			t.Error("checked under mcp serve")
		}
	})
}
