package revive

import (
	"errors"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"shipyard/requests"
	"shipyard/requests/uri"
)

func NewReviveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revive",
		Short: "Revive an environment",
	}

	cmd.AddCommand(newEnvironmentCmd())

	return cmd
}

func newEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"env"},
		Use:     "environment",
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
	client, err := requests.NewClient(os.Stdout)
	if err != nil {
		return err
	}

	body, err := client.Do(http.MethodPost, uri.CreateResourceURI("revive", "environment", id), nil)
	if err != nil {
		return err
	}

	return client.Write(body)
}
