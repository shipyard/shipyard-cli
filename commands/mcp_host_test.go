package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHostConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("SHIPYARD_MCP_RESOURCE_URL", "https://mcp.example.test/mcp")
	t.Setenv("SHIPYARD_MCP_SITE_URL", "")
	t.Setenv("SHIPYARD_MCP_CLIENT_ID", "")
	t.Setenv("SHIPYARD_MCP_CLIENT_SECRET", "host-secret")
	cfg := hostConfigFromEnv()
	if cfg.SiteURL != "https://shipyard.build" {
		t.Errorf("SiteURL = %q", cfg.SiteURL)
	}
	if cfg.ClientID != defaultHostClientID {
		t.Errorf("ClientID = %q", cfg.ClientID)
	}

	t.Setenv("SHIPYARD_MCP_SITE_URL", "https://shipyard.example.test")
	t.Setenv("SHIPYARD_MCP_CLIENT_ID", "custom-host")
	cfg = hostConfigFromEnv()
	if cfg.SiteURL != "https://shipyard.example.test" || cfg.ClientID != "custom-host" {
		t.Errorf("cfg = %+v", cfg)
	}
}

// The hosted server is configured from the environment only. It must start on
// a read-only filesystem, and must not load an operator's CLI config.
func TestMCPHostDoesNotCreateAConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	args := os.Args
	t.Cleanup(func() { os.Args = args })
	os.Args = []string{"shipyard", "mcp", "host", "--listen", ":0"}

	initConfig()

	if _, err := os.Stat(filepath.Join(home, ".shipyard", "config.yaml")); !os.IsNotExist(err) {
		t.Errorf("mcp host created a config file (stat err %v)", err)
	}
}
