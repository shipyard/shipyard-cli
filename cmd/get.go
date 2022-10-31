package cmd

import (
	"github.com/spf13/cobra"

	"shipyard/cmd/env"
	"shipyard/cmd/org"
	"shipyard/cmd/service"
)

func NewGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get information about a resource",
	}

	cmd.AddCommand(env.NewGetAllEnvironmentsCmd())
	cmd.AddCommand(env.NewGetEnvironmentCmd())
	cmd.AddCommand(org.NewGetOrgCmd())
	cmd.AddCommand(org.NewGetAllOrgsCmd())
	cmd.AddCommand(service.NewGetServicesCmd())

	return cmd
}
