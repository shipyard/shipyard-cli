package cluster

import (
	"github.com/spf13/cobra"
)

func NewClusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Manage local shipyard cluster",
		Long:  `Commands for creating, managing, and deleting local shipyard clusters.`,
	}

	return cmd
}
