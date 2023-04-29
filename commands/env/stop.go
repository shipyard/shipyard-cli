package env

import (
	"net/http"

	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/spf13/cobra"
)

func NewStopCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stop",
		GroupID: constants.GroupEnvironments,
		Short:   "Stop a running environment",
		Long:    `This command stops a running environment. You can ONLY stop an environment if it is currently running.`,
		Example: `  # Stop environment ID 12345
  shipyard stop environment 12345`,
	}

	cmd.AddCommand(newStopEnvironmentCmd(c))

	return cmd
}

func newStopEnvironmentCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"env"},
		Use:     "environment [environment ID]",
		Short:   "Stop a running environment",
		Long:    `This command stops a running environment. You can ONLY stop an environment if it is currently running.`,
		Example: `  # Stop environment ID 12345
  shipyard stop environment 12345`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return stopEnvironmentByID(c, args[0])
			}
			return errNoEnvironment
		},
	}

	return cmd
}

func stopEnvironmentByID(c client.Client, id string) error {
	params := make(map[string]string)
	if c.Org != "" {
		params["org"] = c.Org
	}

	_, err := c.Requester.Do(http.MethodPost, uri.CreateResourceURI("stop", "environment", id, "", params), nil)
	if err != nil {
		return err
	}

	display.Println("Environment stopped.")
	return nil
}
