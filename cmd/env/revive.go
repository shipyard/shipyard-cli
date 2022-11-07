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

func NewReviveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "revive",
		GroupID: constants.GroupEnvironments,
		Short:   "Revive an environment",
	}

	cmd.AddCommand(newReviveEnvironmentCmd())

	return cmd
}

func newReviveEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"env"},
		Use:     "environment [environment ID]",
		Short:   "Revive a stopped environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return reviveEnvironmentByID(args[0])
			}
			return errors.New("missing environment ID")
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
