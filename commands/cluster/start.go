package cluster

import (
	"fmt"
	"net/http"
	"os/exec"

	"github.com/fatih/color"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/spf13/cobra"
)

func NewStartCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "start",
		Short:        "Start stopped local shipyard clusters",
		Long:         `Start previously stopped local shipyard clusters`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleStartClusters(c)
		},
	}

	return cmd
}

func handleStartClusters(c *client.Client) error {
	blue := color.New(color.FgHiBlue)
	green := color.New(color.FgHiGreen)

	blue.Println("▶️  Starting Shipyard clusters...")

	// Get list of clusters
	clusters, err := GetShipyardClusters()
	if err != nil {
		return fmt.Errorf("failed to get cluster list: %w", err)
	}

	if len(clusters) == 0 {
		blue.Println("No Shipyard clusters found to start.")
		return nil
	}

	// Start each cluster
	for _, clusterName := range clusters {
		blue.Printf("Starting cluster '%s'...\n", clusterName)

		startCmd := exec.Command("k3d", "cluster", "start", clusterName)
		if err := runCommandWithSpinner(startCmd, "Starting cluster..."); err != nil {
			return fmt.Errorf("failed to start cluster '%s': %w", clusterName, err)
		}

		green.Printf("✓ Cluster '%s' started successfully\n", clusterName)
	}
	startCluster(c)
	return nil
}

func startCluster(c *client.Client) error {
	params := make(map[string]string)
	if org := c.OrgLookupFn(); org != "" {
		params["org"] = org
	}

	// Use CreateResourceURI to build the URL
	url := uri.CreateResourceURI("", "cluster/start", "", "", params)

	// Use the existing HTTP client from the requests package
	_, err := c.Requester.Do(http.MethodGet, url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	return nil
}
