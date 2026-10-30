package commands

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/mcp/hosted"
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

// fakeTokenEndpoint answers every token request with status and an OAuth error.
func fakeTokenEndpoint(t *testing.T, status int, oauthError string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `{"error":%q}`, oauthError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRunHostSetsTheAPIURLFromTheSite(t *testing.T) {
	t.Cleanup(viper.Reset)
	core := fakeTokenEndpoint(t, http.StatusBadRequest, "invalid_grant")
	served := false
	err := runHost(t.Context(), ":0", hosted.Config{
		ResourceURL:  "https://mcp.example.test/mcp",
		SiteURL:      core.URL + "/",
		ClientID:     "host",
		ClientSecret: "host-secret",
	}, func(context.Context, string, http.Handler) error {
		served = true
		return nil
	})
	if err != nil || !served {
		t.Fatalf("err = %v, served = %v", err, served)
	}
	if got := viper.GetString("api_url"); got != core.URL+"/api/v1" {
		t.Errorf("api_url = %q", got)
	}
}

func TestRunHostStopsOnABadConfig(t *testing.T) {
	core := fakeTokenEndpoint(t, http.StatusUnauthorized, "invalid_client")
	for name, cfg := range map[string]hosted.Config{
		"invalid":         {SiteURL: core.URL, ClientID: "host", ClientSecret: "host-secret"},
		"refused by core": {ResourceURL: "https://mcp.example.test/mcp", SiteURL: core.URL, ClientID: "host", ClientSecret: "wrong"},
	} {
		t.Run(name, func(t *testing.T) {
			err := runHost(t.Context(), ":0", cfg, func(context.Context, string, http.Handler) error {
				t.Error("served with a bad config")
				return nil
			})
			if err == nil {
				t.Error("no error")
			}
		})
	}
}

func TestServeHostedStopsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serveHosted(ctx, "127.0.0.1:0", http.NotFoundHandler()) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveHosted didn't return after its context ended")
	}
}

func TestServeHostedReportsAListenError(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = taken.Close() })
	if err := serveHosted(t.Context(), taken.Addr().String(), http.NotFoundHandler()); err == nil {
		t.Error("no error for an address in use")
	}
}
