package get

import (
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"shipyard/requests"
	"shipyard/requests/uri"
)

func newEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Aliases:      []string{"env"},
		Use:          "environment",
		SilenceUsage: true,
		Short:        "Get environments",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return getEnvironmentByID(args[0])
			}
			return getAllEnvironments()
		},
	}

	return cmd
}

func getAllEnvironments() error {
	client, err := requests.NewClient(os.Stdout)
	if err != nil {
		return err
	}

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "environment", ""), nil)
	if err != nil {
		return err
	}

	return client.Write(body)
}

func getEnvironmentByID(id string) error {
	client, err := requests.NewClient(os.Stdout)
	if err != nil {
		return err
	}

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id), nil)
	if err != nil {
		return err
	}

	return client.Write(body)
}
