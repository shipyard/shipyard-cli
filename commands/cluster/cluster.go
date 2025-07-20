package cluster

import (
	"github.com/spf13/cobra"
)

func NewClusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Manage local Kubernetes clusters",
		Long:  `Commands for creating, managing, and deleting local Kubernetes clusters using k3d.`,
	}

	return cmd
}
