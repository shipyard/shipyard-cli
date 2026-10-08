package commands

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/selfupdate"
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

	// upgrade does its own check; completion and __complete run on every Tab.
	for _, name := range []string{"upgrade", "completion", "__complete", "__completeNoDesc", "help"} {
		c := &cobra.Command{Use: name}
		root.AddCommand(c)
		t.Run(name, func(t *testing.T) {
			setup(t)
			if shouldCheckForUpdate(c) {
				t.Errorf("checked under %s", name)
			}
		})
	}
}

func TestShowUpdateNoticeWaitsAtMostNoticeWait(t *testing.T) {
	old := pendingNotice
	t.Cleanup(func() { pendingNotice = old })

	pendingNotice = nil // no check running
	start := time.Now()
	showUpdateNotice()
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("waited %v with no check running", d)
	}

	pendingNotice = make(chan *selfupdate.Notice) // a check that never answers
	start = time.Now()
	showUpdateNotice()
	if d := time.Since(start); d < noticeWait || d > noticeWait+500*time.Millisecond {
		t.Errorf("waited %v for a slow check, want about %v", d, noticeWait)
	}
}

func TestCheckUpgradeTarget(t *testing.T) {
	stable := &selfupdate.Release{TagName: "v1.9.0"}
	for _, c := range []struct {
		current  string
		force    bool
		upToDate bool
		err      bool
	}{
		{current: "1.8.0", upToDate: false},
		{current: "1.9.0", upToDate: true},
		{current: "1.9.0", force: true, upToDate: false},
		{current: "1.10.0-rc.1", upToDate: true},
		{current: "1.10.0-rc.1", force: true, err: true},
		{current: "1.9.1", upToDate: true},
		{current: "1.9.1", force: true, err: true}, // 1.9.1 was withdrawn
	} {
		upToDate, err := checkUpgradeTarget(c.current, stable, c.force)
		if (err != nil) != c.err || upToDate != c.upToDate {
			t.Errorf("current %s force %v: upToDate %v, err %v", c.current, c.force, upToDate, err)
		}
	}
}

func TestCheckUpgradeTargetRejectsTagThatIsntAVersion(t *testing.T) {
	for _, force := range []bool{false, true} {
		if _, err := checkUpgradeTarget("1.9.0", &selfupdate.Release{TagName: "nightly"}, force); err == nil {
			t.Errorf("force %v: installed a release tagged nightly", force)
		}
	}
}

func TestCheckUpgradeTargetAdvice(t *testing.T) {
	stable := &selfupdate.Release{TagName: "v1.9.0", HTMLURL: "https://example.test/v1.9.0"}
	if _, err := checkUpgradeTarget("1.10.0-rc.1", stable, true); err == nil || !strings.Contains(err.Error(), "--prerelease") {
		t.Errorf("pre-release build: %v", err)
	}
	_, err := checkUpgradeTarget("1.9.1", stable, true)
	if err == nil || strings.Contains(err.Error(), "--prerelease") || !strings.Contains(err.Error(), stable.HTMLURL) {
		t.Errorf("withdrawn stable release: %v", err)
	}
}
