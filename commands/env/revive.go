package env

import (
	"net/http"

	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/spf13/cobra"
)

func NewReviveCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "revive",
		GroupID: constants.GroupEnvironments,
		Short:   "Revive a deleted environment",
		Long: `This command revives a deleted environment.
To get the UUID of a deleted environment, you can use:
  shipyard get environments --deleted`,
		Example: `  # Revive environment ID 12345
  shipyard revive environment 12345`,
	}

	cmd.AddCommand(newReviveEnvironmentCmd(c))

	return cmd
}

func newReviveEnvironmentCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"env"},
		Use:     "environment [environment ID]",
		Short:   "Revive a deleted environment",
		Example: `  # Revive environment ID 12345
  shipyard revive environment 12345`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return reviveEnvironmentByID(c, args[0])
			}
			return errNoEnvironment
		},
	}

	return cmd
}

func reviveEnvironmentByID(c client.Client, id string) error {
	params := make(map[string]string)
	if c.Org != "" {
		params["org"] = c.Org
	}

	_, err := c.Requester.Do(http.MethodPost, uri.CreateResourceURI("revive", "environment", id, "", params), nil)
	if err != nil {
		return err
	}

	display.Println("Environment revived.")
	return nil
}
