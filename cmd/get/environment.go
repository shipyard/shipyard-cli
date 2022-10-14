package get

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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
	viper.BindPFlag("name", cmd.Flags().Lookup("name"))

	cmd.Flags().String("org_name", "", "Filter by org name")
	viper.BindPFlag("org_name", cmd.Flags().Lookup("org_name"))

	cmd.Flags().String("repo_name", "", "Filter by repo name")
	viper.BindPFlag("repo_name", cmd.Flags().Lookup("repo_name"))

	cmd.Flags().String("branch", "", "Filter by branch")
	viper.BindPFlag("branch", cmd.Flags().Lookup("branch"))

	cmd.Flags().String("pull_request_number", "", "Filter by pull request number")
	viper.BindPFlag("pull_request_number", cmd.Flags().Lookup("pull_request_number"))

	cmd.Flags().Bool("deleted", false, "Filter by deleted")
	viper.BindPFlag("deleted", cmd.Flags().Lookup("deleted"))

	cmd.Flags().Int("page", 0, "Page number requested")
	viper.BindPFlag("page", cmd.Flags().Lookup("page"))

	cmd.Flags().Int("page_size", 0, "Page size requested")
	viper.BindPFlag("page_size", cmd.Flags().Lookup("page_size"))

	return cmd
}

func getAllEnvironments() error {
	client, err := requests.NewClient(os.Stdout)
	if err != nil {
		return err
	}

	params := make(map[string]string)

	if name := viper.GetString("name"); name != "" {
		params["name"] = name
	}
	if orgName := viper.GetString("org_name"); orgName != "" {
		params["org_name"] = orgName
	}
	if repoName := viper.GetString("repo_name"); repoName != "" {
		params["repo_name"] = repoName
	}
	if branch := viper.GetString("branch"); branch != "" {
		params["branch"] = branch
	}
	if pullRequestNumber := viper.GetString("pull_request_number"); pullRequestNumber != "" {
		params["pull_request_number"] = pullRequestNumber
	}
	if deleted := viper.GetBool("deleted"); deleted {
		params["deleted"] = "true"
	}
	if page := viper.GetInt("page"); page != 0 {
		params["page"] = strconv.Itoa(page)
	}
	if pageSize := viper.GetInt("page_size"); pageSize != 0 {
		params["pageSize"] = strconv.Itoa(pageSize)
	}

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "environment", "", params), nil)
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
