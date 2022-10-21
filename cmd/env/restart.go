package env

import (
	"errors"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"shipyard/requests"
	"shipyard/requests/uri"
)

func NewRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart an environment",
	}

	cmd.AddCommand(newRestartEnvironmentCmd())

	return cmd
}

func newRestartEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Aliases:      []string{"env"},
		Use:          "environment",
		Short:        "Restart a running environment",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return restartEnvironmentByID(args[0])
			}
			return errors.New("missing environment ID")
		},
	}

	return cmd
}

func restartEnvironmentByID(id string) error {
	client, err := requests.NewHTTPClient(os.Stdout)
	if err != nil {
		return err
	}

	body, err := client.Do(http.MethodPost, uri.CreateResourceURI("restart", "environment", id, nil), nil)
	if err != nil {
		return err
	}

	return client.Write(body)
}
