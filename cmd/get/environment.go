package get

import (
	"fmt"
	"net/http"
	"shipyard/requests"
	"shipyard/requests/uri"

	"github.com/spf13/cobra"
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
	client, err := requests.NewHTTPClient()
	if err != nil {
		return err
	}

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "environment", ""), nil)
	if err != nil {
		return err
	}

	fmt.Println(string(body))
	return nil
}

func getEnvironmentByID(id string) error {
	client, err := requests.NewHTTPClient()
	if err != nil {
		return err
	}

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id), nil)
	if err != nil {
		return err
	}

	fmt.Println(string(body))
	return nil
}
