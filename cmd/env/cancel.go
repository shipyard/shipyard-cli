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

func NewCancelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cancel",
		GroupID: constants.GroupEnvironments,
		Short:   "Cancel an environment",
	}

	cmd.AddCommand(newCancelEnvironmentCmd())

	return cmd
}

func newCancelEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"env"},
		Use:     "environment [environment ID]",
		Short:   "Cancel a running environment",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return cancelEnvironmentByID(args[0])
			}
			return errors.New("missing environment ID")
		},
	}

	return cmd
}

func cancelEnvironmentByID(id string) error {
	client, err := requests.NewHTTPClient(os.Stdout)
	if err != nil {
		return err
	}

	params := make(map[string]string)
	org := viper.GetString("org")
	if org != "" {
		params["org"] = org
	}

	body, err := client.Do(http.MethodPost, uri.CreateResourceURI("cancel", "environment", id, params), nil)
	if err != nil {
		return err
	}

	return client.Write(body)
}
