package cluster

import (
	"fmt"
	"os/exec"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func NewStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "stop",
		Short:        "Stop a local shipyard cluster",
		Long:         `Stop a running local shipyard cluster. The cluster will be paused but not deleted.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleStopClusters()
		},
	}

	return cmd
}

func handleStopClusters() error {
	blue := color.New(color.FgHiBlue)
	green := color.New(color.FgHiGreen)

	blue.Println("🛑 Stopping Shipyard clusters...")

	// Get list of clusters
	clusters, err := GetShipyardClusters()
	if err != nil {
		return fmt.Errorf("failed to get cluster list: %w", err)
	}

	if len(clusters) == 0 {
		blue.Println("No Shipyard clusters found to stop.")
		return nil
	}

	// Stop each cluster
	for _, clusterName := range clusters {
		blue.Printf("Stopping cluster '%s'...\n", clusterName)

		stopCmd := exec.Command("k3d", "cluster", "stop", clusterName)
		if err := runCommandWithSpinner(stopCmd, "Stopping cluster..."); err != nil {
			return fmt.Errorf("failed to stop cluster '%s': %w", clusterName, err)
		}

		green.Printf("✓ Cluster '%s' stopped successfully\n", clusterName)
	}
	return nil
}
