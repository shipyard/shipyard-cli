package cmd

import (
	"github.com/spf13/cobra"

	"shipyard/cmd/env"
	"shipyard/cmd/org"
)

func NewGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get information about a resource",
	}

	cmd.AddCommand(env.NewGetAllEnvironmentsCmd())
	cmd.AddCommand(env.NewGetEnvironmentCmd())
	cmd.AddCommand(org.NewGetOrgCmd())

	return cmd
}
