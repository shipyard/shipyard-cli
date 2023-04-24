package commands

import (
	"github.com/spf13/cobra"

	"github.com/shipyard/shipyard-cli/commands/env"
	"github.com/shipyard/shipyard-cli/commands/org"
	"github.com/shipyard/shipyard-cli/commands/services"
	"github.com/shipyard/shipyard-cli/constants"
)

func NewGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		GroupID: constants.GroupEnvironments,
		Short:   "Get information about a resource",
		Example: `  # Get all environments
  shipyard get envs

  # Get environment by ID
  shipyard get environment 12345

  # View all filters available
  shipyard get environments --help
  
  # Get all services in an environment 12345
  shipyard get services --env 12345

  # Get all orgs
  shipyard get orgs`,
	}

	cmd.AddCommand(env.NewGetAllEnvironmentsCmd())
	cmd.AddCommand(env.NewGetEnvironmentCmd())
	cmd.AddCommand(org.NewGetCurrentOrgCmd())
	cmd.AddCommand(org.NewGetAllOrgsCmd())
	cmd.AddCommand(services.NewGetServicesCmd())

	return cmd
}
