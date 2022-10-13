package stop

import (
	"errors"
	"net/http"
	"os"
	"shipyard/requests"
	"shipyard/requests/uri"

	"github.com/spf13/cobra"
)

func NewStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop an environment",
	}

	cmd.AddCommand(newEnvironmentCmd())

	return cmd
}

func newEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"env"},
		Use:     "environment",
		Short:   "Stop a running environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return stopEnvironmentByID(args[0])
			}
			return errors.New("missing environment ID")
		},
	}

	return cmd
}

func stopEnvironmentByID(id string) error {
	client, err := requests.NewClient(os.Stdout)
	if err != nil {
		return err
	}

	body, err := client.Do(http.MethodPost, uri.CreateResourceURI("stop", "environment", id, nil), nil)
	if err != nil {
		return err
	}

	return client.Write(body)
}
