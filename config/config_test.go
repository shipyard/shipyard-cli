package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// The global viper reads SHIPYARD_* env vars. Saving one setting must not write
// those to disk: an MCP client's token would replace the user's own.
func TestSaveWritesOnlyTheGivenKeys(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfg, []byte("api_token: users-own-token\norg: before\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetEnvPrefix("shipyard")
	viper.AutomaticEnv()
	viper.SetDefault("mcp.transport", "stdio")
	viper.SetConfigFile(cfg)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SHIPYARD_API_TOKEN", "client-env-token")
	t.Setenv("SHIPYARD_API_URL", "https://example.invalid/api/v1")

	if err := Save(map[string]any{"org": "after"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	written, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := string(written)

	for _, want := range []string{"users-own-token", "org: after"} {
		if !strings.Contains(got, want) {
			t.Errorf("config is missing %q:\n%s", want, got)
		}
	}
	for _, leaked := range []string{"client-env-token", "example.invalid", "transport"} {
		if strings.Contains(got, leaked) {
			t.Errorf("config picked up %q from env or defaults:\n%s", leaked, got)
		}
	}

	if viper.GetString("org") != "after" {
		t.Errorf("running process sees org %q, want %q", viper.GetString("org"), "after")
	}
}

func TestSaveNeedsAConfigFile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	if err := Save(map[string]any{"org": "x"}); err == nil {
		t.Fatal("expected an error with no config file in use")
	}
}
