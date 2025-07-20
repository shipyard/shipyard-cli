package cluster

import (
	"fmt"
	"os/exec"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func NewDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "delete",
		Short:        "Delete local shipyard clusters",
		Long:         `Delete local shipyard clusters and all their resources. This action cannot be undone.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleDeleteClusters()
		},
	}

	return cmd
}

func handleDeleteClusters() error {
	blue := color.New(color.FgHiBlue)
	green := color.New(color.FgHiGreen)
	red := color.New(color.FgHiRed)

	blue.Println("🗑️  Deleting Shipyard clusters...")

	// Get list of clusters
	clusters, err := GetShipyardClusters()
	if err != nil {
		return fmt.Errorf("failed to get cluster list: %w", err)
	}

	if len(clusters) == 0 {
		blue.Println("No Shipyard clusters found to delete.")
		return nil
	}

	// Show warning about deletion
	red.Printf("⚠️  This will permanently delete %d cluster(s) and all their data!\n", len(clusters))
	for _, clusterName := range clusters {
		red.Printf("   - %s\n", clusterName)
	}

	if !confirmClusterDeletion("all clusters") {
		blue.Println("❌ Cluster deletion cancelled.")
		return nil
	}

	// Delete each cluster
	for _, clusterName := range clusters {
		blue.Printf("Deleting cluster '%s'...\n", clusterName)

		deleteCmd := exec.Command("k3d", "cluster", "delete", clusterName)
		if err := runCommandWithSpinner(deleteCmd, "Deleting cluster..."); err != nil {
			return fmt.Errorf("failed to delete cluster '%s': %w", clusterName, err)
		}

		green.Printf("✓ Cluster '%s' deleted successfully\n", clusterName)
	}
	return nil
}
