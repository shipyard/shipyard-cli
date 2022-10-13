package get

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"shipyard/requests"
	"shipyard/requests/uri"
)

func newGetEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "environment",
		Aliases:      []string{"env"},
		SilenceUsage: true,
		Short:        "Get environment by ID",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return getEnvironmentByID(args[0])
			}
			return fmt.Errorf("missing ID argument")
		},
	}

	return cmd
}

func newGetAllEnvironmentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "environments",
		Aliases:      []string{"envs"},
		SilenceUsage: true,
		Short:        "Get all environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return getAllEnvironments()
		},
	}

	cmd.Flags().String("name", "", "Filter by name")
	cmd.Flags().String("org_name", "", "Filter by org name")
	cmd.Flags().String("repo_name", "", "Filter by repo name")
	cmd.Flags().String("branch", "", "Filter by branch")
	cmd.Flags().String("pull_request_number", "", "Filter by pull request number")
	cmd.Flags().Bool("deleted", false, "Filter by deleted")
	cmd.Flags().Int("page", 0, "Page number requested")
	cmd.Flags().Int("page_size", 0, "Page size requested")

	return cmd
}

func getAllEnvironments() error {
	client, err := requests.NewClient(os.Stdout)
	if err != nil {
		return err
	}

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "environment", "", nil), nil)
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

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id, nil), nil)
	if err != nil {
		return err
	}

	return client.Write(body)
}
