package restart

import (
	"errors"
	"fmt"
	"net/http"
	"shipyard/requests"
	"shipyard/requests/uri"

	"github.com/spf13/cobra"
)

func NewRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart an environment",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	}

	cmd.AddCommand(newEnvironmentCmd())

	return cmd
}

func newEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"env"},
		Use:     "environment",
		Short:   "Restart a running environment",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
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
	client, err := requests.NewHTTPClient()
	if err != nil {
		return err
	}

	body, err := client.Do(http.MethodPost, uri.CreateResourceURI("restart", "environment", id), nil)
	if err != nil {
		return err
	}

	fmt.Println(string(body))
	return nil
}
