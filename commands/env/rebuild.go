package env

import (
	"net/http"

	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/spf13/cobra"
)

func NewRebuildCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rebuild",
		GroupID: constants.GroupEnvironments,
		Short:   "Rebuild an environment",
		Long: `This command rebuilds an environment. You can only rebuild a non-deleted environment.
Rebuild will automatically fetch the latest commit for the branch/PR.`,
		Example: `  # Rebuild environment ID 12345
  shipyard rebuild environment 12345`,
	}

	cmd.AddCommand(newRebuildEnvironmentCmd(c))

	return cmd
}

func newRebuildEnvironmentCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"env"},
		Use:     "environment [environment ID]",
		Short:   "Rebuild an environment",
		Long: `This command rebuilds an environment. You can only rebuild a non-deleted environment.
Rebuild will automatically fetch the latest commit for the branch/PR.`,
		Example: `  # Rebuild environment ID 12345
  shipyard rebuild environment 12345`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return rebuildEnvironmentByID(c, args[0])
			}
			return errNoEnvironment
		},
	}

	return cmd
}

func rebuildEnvironmentByID(c client.Client, id string) error {
	params := make(map[string]string)
	if c.Org != "" {
		params["org"] = c.Org
	}

	_, err := c.Requester.Do(http.MethodPost, uri.CreateResourceURI("rebuild", "environment", id, "", params), "application/json", nil)
	if err != nil {
		return err
	}

	display.Println("Environment queued for a rebuild.")
	return nil
}
