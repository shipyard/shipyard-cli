package env

import (
	"errors"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/requests"
	"github.com/shipyard/shipyard-cli/requests/uri"
)

func NewReviveCmd() *cobra.Command {
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

	cmd.AddCommand(newReviveEnvironmentCmd())

	return cmd
}

func newReviveEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"env"},
		Use:     "environment [environment ID]",
		Short:   "Revive a deleted environment",
		Example: `  # Revive environment ID 12345
  shipyard revive environment 12345`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return reviveEnvironmentByID(args[0])
			}
			return errors.New("environment ID argument not provided")
		},
	}

	return cmd
}

func reviveEnvironmentByID(id string) error {
	client, err := requests.NewHTTPClient(os.Stdout)
	if err != nil {
		return err
	}

	params := make(map[string]string)
	org := viper.GetString("org")
	if org != "" {
		params["org"] = org
	}

	body, err := client.Do(http.MethodPost, uri.CreateResourceURI("revive", "environment", id, "", params), nil)
	if err != nil {
		return err
	}

	return client.Write(body)
}
