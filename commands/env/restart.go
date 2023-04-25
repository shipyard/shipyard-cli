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

func NewRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "restart",
		GroupID: constants.GroupEnvironments,
		Short:   "Restart a stopped environment",
		Example: `  # Restart environment ID 12345
  shipyard restart environment 12345`,
		SilenceUsage: true,
	}

	cmd.AddCommand(newRestartEnvironmentCmd())

	return cmd
}

func newRestartEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Aliases:      []string{"env"},
		Use:          "environment [environment ID]",
		SilenceUsage: true,
		Short:        "Restart a stopped environment",
		Example: `  # Restart environment ID 12345
  shipyard restart environment 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return restartEnvironmentByID(args[0])
			}
			return errNoEnvironment
		},
	}

	return cmd
}

func restartEnvironmentByID(id string) error {
	requester, err := requests.New(io.Discard)
	if err != nil {
		return err
	}

	params := make(map[string]string)
	org := viper.GetString("org")
	if org != "" {
		params["org"] = org
	}

	_, err = requester.Do(http.MethodPost, uri.CreateResourceURI("restart", "environment", id, "", params), nil)
	if err != nil {
		return err
	}

	out := display.New()
	out.Println("Environment queued for a restart.")
	return nil
}
