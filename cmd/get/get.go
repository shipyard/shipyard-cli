package get

import (
	"github.com/spf13/cobra"
)

func NewGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get information about a resource",
	}

	cmd.AddCommand(newGetAllEnvironmentsCmd())
	cmd.AddCommand(newGetEnvironmentCmd())

	return cmd
}
