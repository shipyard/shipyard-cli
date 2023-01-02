package cmd

import (
	"github.com/spf13/cobra"

	"shipyard/cmd/env"
	"shipyard/cmd/org"
	"shipyard/cmd/services"
	"shipyard/constants"
)

func NewGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		GroupID: constants.GroupEnvironments,
		Short:   "Get information about a resource",
		Example: `  # Get all environments
  shipyard get environments --env 12345

  # View all filters available
  shipyard get environments --help

  # Get environment by ID
  shipyard get environment --env 12345
  
  # Get all services in an environment 12345
  shipyard get services --env 12345`,
	}

	cmd.AddCommand(env.NewGetAllEnvironmentsCmd())
	cmd.AddCommand(env.NewGetEnvironmentCmd())
	cmd.AddCommand(org.NewGetCurrentOrgCmd())
	cmd.AddCommand(org.NewGetAllOrgsCmd())
	cmd.AddCommand(services.NewGetServicesCmd())

	return cmd
}
