package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// newModeRoot builds a root with mode and a get command that records what reached it.
func newModeRoot(t *testing.T) (root *cobra.Command, ran *bool, token *bool, org *string, apiURL *string) {
	t.Helper()
	t.Setenv("SHIPYARD_API_URL", "")
	t.Setenv("SHIPYARD_API_TOKEN", "")

	// Execute runs initConfig, which would load the real config into the global viper
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfgFile = configFile
	t.Cleanup(func() {
		cfgFile = ""
		viper.SetConfigFile("")
	})

	ran, token, org, apiURL = new(bool), new(bool), new(string), new(string)
	get := &cobra.Command{
		Use: "get",
		RunE: func(cmd *cobra.Command, _ []string) error {
			*ran = true
			*apiURL = os.Getenv("SHIPYARD_API_URL")
			return nil
		},
	}
	get.Flags().BoolVar(token, "bypass-token", false, "")

	root = &cobra.Command{Use: "shipyard", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().StringVar(org, "org", "", "")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.AddCommand(NewModeCmd(), get)
	return root, ran, token, org, apiURL
}

func TestModePassesFlagsToTheCommand(t *testing.T) {
	tests := map[string][]string{
		"flags after the command":  {"mode", "qa", "get", "--org", "acme", "--bypass-token"},
		"flags before the command": {"mode", "qa", "--org", "acme", "get", "--bypass-token"},
		"separator after the mode": {"mode", "qa", "--", "get", "--org", "acme", "--bypass-token"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			root, ran, token, org, apiURL := newModeRoot(t)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !*ran {
				t.Fatal("the command didn't run")
			}
			if !*token {
				t.Error("--bypass-token didn't reach the command")
			}
			if *org != "acme" {
				t.Errorf("org = %q, want acme", *org)
			}
			if *apiURL != "https://qa.shipyard.build/api/v1" {
				t.Errorf("SHIPYARD_API_URL = %q during the command", *apiURL)
			}
		})
	}
}

func TestModeHelpIsItsOwn(t *testing.T) {
	root, ran, _, _, _ := newModeRoot(t)
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"mode", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if *ran {
		t.Error("--help ran a command")
	}
	if !bytes.Contains(out.Bytes(), []byte("shipyard mode qa")) {
		t.Errorf("help is not mode's:\n%s", out)
	}
}
