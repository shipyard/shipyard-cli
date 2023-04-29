package commands

import (
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/spf13/cobra"

	"github.com/shipyard/shipyard-cli/commands/env"
	"github.com/shipyard/shipyard-cli/commands/org"
	"github.com/shipyard/shipyard-cli/commands/services"
	"github.com/shipyard/shipyard-cli/constants"
)

func NewGetCmd(c client.Client) *cobra.Command {
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

	cmd.AddCommand(env.NewGetAllEnvironmentsCmd(c))
	cmd.AddCommand(env.NewGetEnvironmentCmd(c))
	cmd.AddCommand(org.NewGetCurrentOrgCmd())
	cmd.AddCommand(org.NewGetAllOrgsCmd(c))
	cmd.AddCommand(services.NewGetServicesCmd(c))

	return cmd
}
