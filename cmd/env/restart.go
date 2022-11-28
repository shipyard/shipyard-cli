package env

import (
	"errors"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"shipyard/constants"
	"shipyard/requests"
	"shipyard/requests/uri"
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
		Aliases: []string{"env"},
		Use:     "environment [environment ID]",
		Short:   "Restart a stopped environment",
		Example: `  # Restart environment ID 12345
  shipyard restart environment 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return restartEnvironmentByID(args[0])
			}
			return errors.New("Environment ID argument not provided")
		},
	}

	return cmd
}

func restartEnvironmentByID(id string) error {
	client, err := requests.NewHTTPClient(os.Stdout)
	if err != nil {
		return err
	}

	params := make(map[string]string)
	org := viper.GetString("org")
	if org != "" {
		params["org"] = org
	}

	body, err := client.Do(http.MethodPost, uri.CreateResourceURI("restart", "environment", id, "", params), nil)
	if err != nil {
		return err
	}

	return client.Write(body)
}
