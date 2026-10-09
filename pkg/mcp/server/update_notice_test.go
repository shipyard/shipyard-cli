package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/selfupdate"
	"github.com/shipyard/shipyard-cli/version"
)

func TestStaleNotice(t *testing.T) {
	tests := []struct {
		name, current, latest string
		want                  bool
	}{
		{"newer release cached", "1.9.0", "1.10.0", true},
		{"same version", "1.10.0", "1.10.0", false},
		{"cached is older", "1.10.0", "1.9.0", false},
		{"nothing cached", "1.9.0", "", false},
		{"dev build", "undefined", "1.10.0", false},
		{"snapshot build", "1.10.0-SNAPSHOT-abc", "1.11.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := staleNotice(tt.current, tt.latest)
			if (got != "") != tt.want {
				t.Fatalf("staleNotice(%q, %q) = %q, want notice: %v", tt.current, tt.latest, got, tt.want)
			}
			if tt.want {
				for _, s := range []string{tt.current, tt.latest, "shipyard upgrade"} {
					if !strings.Contains(got, s) {
						t.Errorf("notice %q is missing %q", got, s)
					}
				}
			}
		})
	}
}

// A stale server says so where the agent reads it: clients hide stderr, and the
// CLI's own update notice never runs under `mcp serve`.
func TestHandleInitialize_StaleNotice(t *testing.T) {
	old := version.Version
	version.Version = "1.9.0"
	t.Cleanup(func() { version.Version = old })

	instructionsFor := func(latest string) string {
		s := NewMCPServer(MCPServerConfig{LatestVersion: latest}, newMockClient())
		resp := s.handleInitialize(&JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"})
		var r struct {
			Result struct {
				Instructions string `json:"instructions"`
			} `json:"result"`
		}
		if err := json.Unmarshal(resp, &r); err != nil {
			t.Fatal(err)
		}
		return r.Result.Instructions
	}

	if got := instructionsFor(""); got != Instructions() {
		t.Errorf("up to date: instructions changed:\n%s", got)
	}
	got := instructionsFor("1.10.0")
	// First, because clients truncate long instructions.
	if !strings.HasPrefix(got, staleNotice("1.9.0", "1.10.0")) || !strings.HasSuffix(got, Instructions()) {
		t.Errorf("stale: expected the upgrade notice followed by the instructions, got:\n%s", got)
	}
}

// The cached result of the CLI's last update check is read from disk, never
// fetched: startup must not wait on GitHub. The CLI's opt-outs apply.
func TestLoadMCPServerConfig_LatestVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".shipyard"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := (selfupdate.State{LatestVersion: "1.10.0"}).Save(selfupdate.StatePath(home)); err != nil {
		t.Fatal(err)
	}

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetDefault("update_check", true)

	if got := LoadMCPServerConfig().LatestVersion; got != "1.10.0" {
		t.Errorf("LatestVersion = %q, want 1.10.0", got)
	}

	t.Setenv("SHIPYARD_NO_UPDATE_CHECK", "1")
	if got := LoadMCPServerConfig().LatestVersion; got != "" {
		t.Errorf("SHIPYARD_NO_UPDATE_CHECK set: LatestVersion = %q, want empty", got)
	}
	t.Setenv("SHIPYARD_NO_UPDATE_CHECK", "")

	viper.Set("update_check", false)
	if got := LoadMCPServerConfig().LatestVersion; got != "" {
		t.Errorf("update_check off: LatestVersion = %q, want empty", got)
	}
}

// Claude Code drops everything past 2048 characters of the instructions, so the
// notice and the instructions together must fit, even with long versions.
func TestStaleNotice_FitsWithInstructions(t *testing.T) {
	const clientLimit = 2048
	notice := staleNotice("10.10.10", "10.10.11")
	if size := len(notice) + len("\n\n") + len(Instructions()); size > clientLimit {
		t.Errorf("notice (%d) plus instructions (%d) is %d bytes, over Claude Code's %d-character limit",
			len(notice), len(Instructions()), size, clientLimit)
	}
}
