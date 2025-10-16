package commands

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/mcp/server"
)

// NewMCPCmd creates the MCP command group
func NewMCPCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol server operations",
		Long:  "Start and manage the MCP server for AI assistant integration",
	}

	cmd.AddCommand(NewMCPServeCmd(c))
	return cmd
}

// NewMCPServeCmd creates the MCP serve command
func NewMCPServeCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP server",
		Long: `Start the Model Context Protocol server to enable AI assistants
to interact with Shipyard environments through standardized tools.`,
		Example: `  # Start MCP server with stdio transport (default)
  shipyard mcp serve

`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The --org flag is intentionally not supported for MCP commands because:
			// 1. MCP servers are typically long-running processes that shouldn't change org context
			// 2. The org should be configured once via environment variables or config file
			// 3. This prevents confusion about which org context the MCP server is operating in
			if cmd.Flags().Changed("org") || (cmd.Parent() != nil && cmd.Parent().PersistentFlags().Changed("org")) {
				return fmt.Errorf("the --org flag is not supported for MCP commands; use environment variable SHIPYARD_ORG or set org in config file instead")
			}
			return runMCPServe(c)
		},
	}

	return cmd
}

// runMCPServe starts the MCP server
func runMCPServe(c client.Client) error {
	// Load MCP server configuration
	config := server.LoadMCPServerConfig()

	// Create MCP server
	mcpServer := server.NewMCPServer(config, c)

	// Start server
	if err := mcpServer.Start(); err != nil {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("MCP server running. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutting down MCP server...")

	// Stop server
	if err := mcpServer.Stop(); err != nil {
		return fmt.Errorf("error stopping MCP server: %w", err)
	}

	log.Println("MCP server stopped successfully")
	return nil
}
