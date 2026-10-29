package commands

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/mcp/hosted"
)

const defaultHostClientID = "shipyard-mcp-host"

// NewMCPHostCmd runs the hosted MCP server: Streamable HTTP for clients that
// sign in with OAuth. It never uses the CLI's own API token or config.
func NewMCPHostCmd() *cobra.Command {
	var listen string
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Run the hosted MCP server over HTTP",
		Long: `Run the hosted MCP server: stateless Streamable HTTP for MCP clients that connect
with OAuth. Each request's token is exchanged for a short-lived Shipyard API token.

It is configured from the environment only, so the client secret never lands in a config file:

  SHIPYARD_MCP_RESOURCE_URL   this server's MCP URL, e.g. https://mcp.example.com/mcp (required)
  SHIPYARD_MCP_SITE_URL       Shipyard's URL (default https://shipyard.build)
  SHIPYARD_MCP_CLIENT_ID      this server's OAuth client id (default shipyard-mcp-host)
  SHIPYARD_MCP_CLIENT_SECRET  this server's OAuth client secret (required)`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := hostConfigFromEnv()
			handler, err := hosted.Handler(cfg)
			if err != nil {
				return fmt.Errorf("the hosted MCP server is not configured: %w", err)
			}
			// The tools build API URLs from api_url; it has to be the API the
			// exchanged tokens are for. Set in memory only, never written back.
			viper.Set("api_url", strings.TrimSuffix(cfg.SiteURL, "/")+"/api/v1")
			return serveHosted(cmd.Context(), listen, handler)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", ":8080", "address to listen on")
	return cmd
}

func hostConfigFromEnv() hosted.Config {
	cfg := hosted.Config{
		ResourceURL:  os.Getenv("SHIPYARD_MCP_RESOURCE_URL"),
		SiteURL:      os.Getenv("SHIPYARD_MCP_SITE_URL"),
		ClientID:     os.Getenv("SHIPYARD_MCP_CLIENT_ID"),
		ClientSecret: os.Getenv("SHIPYARD_MCP_CLIENT_SECRET"),
	}
	if cfg.SiteURL == "" {
		cfg.SiteURL = "https://shipyard.build"
	}
	if cfg.ClientID == "" {
		cfg.ClientID = defaultHostClientID
	}
	return cfg
}

func serveHosted(ctx context.Context, listen string, handler http.Handler) error {
	// A server's logs are for its operator, whatever --verbose says.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.LUTC)

	srv := &http.Server{
		Addr:              listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// Tool calls wait on the API, which gives up after 20 seconds.
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  2 * time.Minute,
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		log.Printf("hosted MCP server listening on %s", listen)
		errs <- srv.ListenAndServe()
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	log.Println("hosted MCP server shutting down")
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
