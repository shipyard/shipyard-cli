package env

import (
	"io"
	"net/http"

	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/constants"
)

func NewCancelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cancel",
		GroupID: constants.GroupEnvironments,
		Short:   "Cancel an environment's latest build",
		Long:    `This command cancels the environment's latest build. You can ONLY cancel a build if it is currently in the building phase.`,
		Example: `  # Cancel the current build for environment ID 12345
  shipyard cancel environment 12345`,
	}

	cmd.AddCommand(newCancelEnvironmentCmd())

	return cmd
}

func newCancelEnvironmentCmd() *cobra.Command {
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
				return cancelEnvironmentByID(args[0])
			}
			return errNoEnvironment
		},
	}

	return cmd
}

func cancelEnvironmentByID(id string) error {
	requester, err := requests.New(io.Discard)
	if err != nil {
		return err
	}

	params := make(map[string]string)
	org := viper.GetString("org")
	if org != "" {
		params["org"] = org
	}

	_, err = requester.Do(http.MethodPost, uri.CreateResourceURI("cancel", "environment", id, "", params), nil)
	if err != nil {
		return err
	}

	display.Println("Environment canceled.")
	return nil
}
