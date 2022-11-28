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

func NewRebuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rebuild",
		GroupID: constants.GroupEnvironments,
		Short:   "Rebuild an environment",
		Long: `This command rebuilds an environment. You can only rebuild a non-deleted environment.
Rebuild will automatically fetch the latest commit for the branch/PR.`,
		Example: `  # Rebuild environment ID 12345
  shipyard rebuild environment 12345`,
	}

	cmd.AddCommand(newRebuildEnvironmentCmd())

	return cmd
}

func newRebuildEnvironmentCmd() *cobra.Command {
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
				return rebuildEnvironmentByID(args[0])
			}
			return errors.New("Environment ID argument not provided")
		},
	}

	return cmd
}

func rebuildEnvironmentByID(id string) error {
	client, err := requests.NewHTTPClient(os.Stdout)
	if err != nil {
		return err
	}

	params := make(map[string]string)
	org := viper.GetString("org")
	if org != "" {
		params["org"] = org
	}

	body, err := client.Do(http.MethodPost, uri.CreateResourceURI("rebuild", "environment", id, "", params), nil)
	if err != nil {
		return err
	}

	return client.Write(body)
}
