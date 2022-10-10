package get

import (
	"github.com/spf13/cobra"
)

func NewGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get information about something",
	}

	cmd.AddCommand(newEnvironmentCmd())

	return cmd
}
