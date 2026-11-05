package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const qaURL = "https://qa.shipyard.build/api/v1"

// modeRun records what reached the command mode ran.
type modeRun struct {
	ran      bool
	token    bool
	org      string
	apiURL   string
	apiToken string
}

// execute runs args as the command line: initConfig reads os.Args, not root's args.
func execute(t *testing.T, root *cobra.Command, args ...string) error {
	t.Helper()
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	os.Args = append([]string{"shipyard"}, args...)
	root.SetArgs(args)
	return root.Execute()
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// newModeRoot builds a root with mode and a get command that records what reached it.
func newModeRoot(t *testing.T) (*cobra.Command, *modeRun) {
	t.Helper()
	t.Setenv("SHIPYARD_API_URL", "")
	t.Setenv("SHIPYARD_API_TOKEN", "")

	// Execute runs initConfig, which would load the real config into the global viper
	cfgFile = writeConfig(t, "")
	t.Cleanup(func() {
		cfgFile = ""
		viper.SetConfigFile("")
		// config.Save sets what it saves on the global viper too
		viper.Set("api_url", nil)
	})

	run := &modeRun{}
	get := &cobra.Command{
		Use: "get",
		RunE: func(*cobra.Command, []string) error {
			run.ran = true
			run.apiURL = os.Getenv("SHIPYARD_API_URL")
			run.apiToken = os.Getenv("SHIPYARD_API_TOKEN")
			return nil
		},
	}
	get.Flags().BoolVar(&run.token, "bypass-token", false, "")
	get.Flags().StringArray("field", nil, "")

	root := &cobra.Command{Use: "shipyard", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().StringVar(&run.org, "org", "", "")
	root.PersistentFlags().StringVar(&cfgFile, "config", cfgFile, "")
	root.PersistentFlags().BoolP("verbose", "v", false, "")
	root.PersistentFlags().StringP("filter", "f", "", "")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.AddCommand(NewModeCmd(), get)
	return root, run
}

func TestModePassesFlagsToTheCommand(t *testing.T) {
	tests := map[string][]string{
		"flags after the command":  {"mode", "qa", "get", "--org", "acme", "--bypass-token"},
		"flags before the command": {"mode", "qa", "--org", "acme", "get", "--bypass-token"},
		"separator after the mode": {"mode", "qa", "--", "get", "--org", "acme", "--bypass-token"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			root, run := newModeRoot(t)
			if err := execute(t, root, args...); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !run.ran {
				t.Fatal("the command didn't run")
			}
			if !run.token {
				t.Error("--bypass-token didn't reach the command")
			}
			if run.org != "acme" {
				t.Errorf("org = %q, want acme", run.org)
			}
			if run.apiURL != qaURL {
				t.Errorf("SHIPYARD_API_URL = %q during the command", run.apiURL)
			}
		})
	}
}

// The profile's token overrides the config for the command, so a named config
// has to be read before the profile is picked, or the command runs as someone else.
func TestModeTakesTheTokenFromANamedConfig(t *testing.T) {
	named := writeConfig(t, "profiles:\n  qa:\n    auth_token: named-token\n")
	tests := map[string][]string{
		"after the command":   {"mode", "qa", "get", "--config", named},
		"before the command":  {"mode", "qa", "--config", named, "get"},
		"with an equals sign": {"mode", "qa", "get", "--config=" + named},
		"after a separator":   {"mode", "qa", "--", "get", "--config", named},
		// The h in an attached short flag's value isn't a request for help
		"after a short flag":             {"mode", "qa", "get", "-fbranch=main", "--config", named},
		"after flags and a separator":    {"mode", "qa", "-v", "--", "get", "--config", named},
		"after a separator, flag before": {"--org", "acme", "mode", "qa", "--", "get", "--config", named},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			root, run := newModeRoot(t)
			if err := os.WriteFile(cfgFile, []byte("profiles:\n  qa:\n    auth_token: default-token\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := execute(t, root, args...); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if run.apiToken != "named-token" {
				t.Errorf("token = %q, want the named config's", run.apiToken)
			}
		})
	}
}

// What only looks like --config isn't one: it's a flag's value, or belongs to
// what the command runs. Reading it would take the token from another file.
func TestModeOnlyReadsARealConfigFlag(t *testing.T) {
	named := writeConfig(t, "profiles:\n  qa:\n    auth_token: named-token\n")
	tests := map[string][]string{
		"a flag's value":                {"mode", "qa", "get", "--field", "--config=" + named},
		"after the command's separator": {"mode", "qa", "get", "--", "prog", "--config", named},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			root, run := newModeRoot(t)
			if err := os.WriteFile(cfgFile, []byte("profiles:\n  qa:\n    auth_token: default-token\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := execute(t, root, args...); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if run.apiToken != "default-token" {
				t.Errorf("token = %q, want the default config's", run.apiToken)
			}
		})
	}
}

func TestModeDoesntNeedTheDefaultConfigWhenOneIsNamed(t *testing.T) {
	named := writeConfig(t, "profiles:\n  qa:\n    auth_token: named-token\n")
	root, run := newModeRoot(t)
	if err := os.WriteFile(cfgFile, []byte("not: [yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := execute(t, root, "mode", "qa", "get", "--config", named); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if run.apiToken != "named-token" {
		t.Errorf("token = %q, want the named config's", run.apiToken)
	}
}

// Cobra's search for the command stops at the separator, so flags before it
// mustn't read as a mode with no command: that would rewrite the saved
// endpoint and token, and skip what was asked for.
func TestModeRunsACommandAfterFlagsAndASeparator(t *testing.T) {
	tests := map[string][]string{
		"a bool flag":  {"mode", "qa", "-v", "--", "get", "--bypass-token"},
		"a value flag": {"mode", "qa", "--org", "acme", "--", "get", "--bypass-token"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			root, run := newModeRoot(t)
			if err := execute(t, root, args...); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !run.ran {
				t.Error("the command didn't run")
			}
			if !run.token {
				t.Error("--bypass-token didn't reach the command")
			}
			if run.apiURL != qaURL {
				t.Errorf("SHIPYARD_API_URL = %q during the command", run.apiURL)
			}
			wantSavedMode(t, cfgFile, false)
		})
	}
}

func TestModeWithAnUnknownCommandSavesNothing(t *testing.T) {
	root, _ := newModeRoot(t)
	if err := execute(t, root, "mode", "qa", "nope"); err == nil {
		t.Fatal("no error")
	}
	wantSavedMode(t, cfgFile, false)
}

func wantSavedMode(t *testing.T, path string, want bool) {
	t.Helper()
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Contains(string(saved), qaURL); got != want {
		t.Errorf("mode saved = %v, want %v:\n%s", got, want, saved)
	}
}

// Without a command there's nothing to run, so the mode has to stick: a script
// that goes on would otherwise reach whichever environment was set before.
func TestModeWithOnlyFlagsSetsTheMode(t *testing.T) {
	tests := map[string][]string{
		"a bool flag":  {"mode", "qa", "-v"},
		"a value flag": {"mode", "qa", "--org", "acme"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			root, run := newModeRoot(t)
			if err := execute(t, root, args...); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if run.ran {
				t.Error("a command ran")
			}
			wantSavedMode(t, cfgFile, true)
		})
	}
}

func TestModeWithOnlyANamedConfigSavesThere(t *testing.T) {
	named := writeConfig(t, "")
	root, _ := newModeRoot(t)
	defaultConfig := cfgFile
	if err := execute(t, root, "mode", "qa", "--config", named); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	wantSavedMode(t, named, true)
	wantSavedMode(t, defaultConfig, false)
}

func TestModeSavesToAConfigNamedAfterASeparator(t *testing.T) {
	named := writeConfig(t, "")
	root, _ := newModeRoot(t)
	defaultConfig := cfgFile
	if err := execute(t, root, "-v", "mode", "qa", "--", "--config", named); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	wantSavedMode(t, named, true)
	wantSavedMode(t, defaultConfig, false)
}

func TestModeHelpDoesntNeedAConfig(t *testing.T) {
	root, _ := newModeRoot(t)
	if err := os.WriteFile(cfgFile, []byte("not: [yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := execute(t, root, "mode", "qa", "--help"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("shipyard mode qa")) {
		t.Errorf("help is not mode's:\n%s", out)
	}
}

func TestModeRefusesAFlagNothingKnows(t *testing.T) {
	root, _ := newModeRoot(t)
	if err := execute(t, root, "mode", "qa", "--nope"); err == nil {
		t.Fatal("no error")
	}
	wantSavedMode(t, cfgFile, false)
}

func TestModeHelpIsItsOwn(t *testing.T) {
	tests := map[string][]string{
		"before the mode": {"mode", "--help"},
		"after the mode":  {"mode", "qa", "--help"},
		"short":           {"mode", "qa", "-h"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			root, run := newModeRoot(t)
			out := &bytes.Buffer{}
			root.SetOut(out)
			if err := execute(t, root, args...); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if run.ran {
				t.Error("--help ran a command")
			}
			if !bytes.Contains(out.Bytes(), []byte("shipyard mode qa")) {
				t.Errorf("help is not mode's:\n%s", out)
			}
			wantSavedMode(t, cfgFile, false)
		})
	}
}
