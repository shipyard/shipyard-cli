package cmd

import (
	"github.com/spf13/cobra"

	"shipyard/cmd/org"
)

func NewSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set a resource locally",
	}

	cmd.AddCommand(org.NewSetOrgCmd())

	return cmd
}
