package env

import (
	"net/http"

	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/spf13/cobra"
)

func NewCancelCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cancel",
		GroupID: constants.GroupEnvironments,
		Short:   "Cancel an environment's latest build",
		Long:    `This command cancels the environment's latest build. You can ONLY cancel a build if it is currently in the building phase.`,
		Example: `  # Cancel the current build for environment ID 12345
  shipyard cancel environment 12345`,
	}

	cmd.AddCommand(newCancelEnvironmentCmd(c))

	return cmd
}

func newCancelEnvironmentCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Aliases:      []string{"env"},
		Use:          "environment [environment ID]",
		SilenceUsage: true,
		Short:        "Cancel an environment's latest build",
		Long:         `This command cancels the environment's latest build. You can ONLY cancel a build if it is currently in the building phase.`,
		Example: `  # Cancel the current build for environment ID 12345
  shipyard cancel environment 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return cancelEnvironmentByID(c, args[0])
			}
			return errNoEnvironment
		},
	}

	return cmd
}

func cancelEnvironmentByID(c client.Client, id string) error {
	params := make(map[string]string)
	if c.Org != "" {
		params["org"] = c.Org
	}
	_, err := c.Requester.Do(http.MethodPost, uri.CreateResourceURI("cancel", "environment", id, "", params), nil)
	if err != nil {
		return err
	}

	display.Println("Environment canceled.")
	return nil
}
