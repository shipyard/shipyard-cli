package env

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/completion"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/types"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var errNoEnvironment = errors.New("environment ID argument not provided")

func NewGetEnvironmentCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "environment [environment ID]",
		Aliases: []string{"env"},
		Short:   "Get an environment's details by ID",
		Example: `  # Get all the details for environment ID 12345:
  shipyard get environment 12345
  
  # Get all the details for environment ID 12345 in JSON format:
  shipyard get environment 12345 --json`,
		SilenceUsage: true,
		// Due to an issue in viper, bind the 'json' flag in PreRun for each command that uses
		// a flag name already bound to a sibling command.
		// See https://github.com/spf13/viper/issues/233#issuecomment-386791444
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("json", cmd.Flags().Lookup("json"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return handleGetEnvironmentByID(c, args[0])
			}
			return errNoEnvironment
		},
		ValidArgsFunction: completion.New(c).EnvironmentUUIDs,
	}

	cmd.Flags().Bool("json", false, "JSON output")

	return cmd
}

func NewGetAllEnvironmentsCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "environments",
		Aliases:      []string{"envs"},
		SilenceUsage: true,
		Short:        "Get details for all environments in an org",
		Example: `  # Get details on all environments in your default org:
  shipyard get environments
  
  # Get all the details in JSON format:
  shipyard get environments --json
  
  # Get all the environments for a specific repo and branch:
  shipyard get environments --repo-name flask-backend --branch main
  
  # Get all the environments based on specific PR:
  shipyard get environments --pull-request-number 1
  `,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("name", cmd.Flags().Lookup("name"))
			_ = viper.BindPFlag("org-name", cmd.Flags().Lookup("org-name"))
			_ = viper.BindPFlag("repo-name", cmd.Flags().Lookup("repo-name"))
			_ = viper.BindPFlag("branch", cmd.Flags().Lookup("branch"))
			_ = viper.BindPFlag("pull-request-number", cmd.Flags().Lookup("pull-request-number"))
			_ = viper.BindPFlag("deleted", cmd.Flags().Lookup("deleted"))
			_ = viper.BindPFlag("page", cmd.Flags().Lookup("page"))
			_ = viper.BindPFlag("page-size", cmd.Flags().Lookup("page-size"))
			_ = viper.BindPFlag("json", cmd.Flags().Lookup("json"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleGetAllEnvironments(c)
		},
	}

	cmd.Flags().String("name", "", "Filter by name of the application")
	cmd.Flags().String("org-name", "", "Filter by org name")
	cmd.Flags().String("repo-name", "", "Filter by repo name")
	cmd.Flags().String("branch", "", "Filter by branch name")
	cmd.Flags().String("pull-request-number", "", "Filter by pull request number")
	cmd.Flags().Bool("deleted", false, "Filter by deleted status (default false)")
	cmd.Flags().Int("page", 1, "Page number requested")
	cmd.Flags().Int("page-size", 20, "Page size requested")
	cmd.Flags().Bool("json", false, "JSON output")

	return cmd
}

//nolint:gocyclo // refactor?
func handleGetAllEnvironments(c client.Client) error {
	params := make(map[string]string)

	if name := viper.GetString("name"); name != "" {
		params["name"] = name
	}
	if orgName := viper.GetString("org-name"); orgName != "" {
		params["org_name"] = orgName
	}
	if repoName := viper.GetString("repo-name"); repoName != "" {
		params["repo_name"] = repoName
	}
	if branch := viper.GetString("branch"); branch != "" {
		params["branch"] = branch
	}
	if pullRequestNumber := viper.GetString("pull-request-number"); pullRequestNumber != "" {
		params["pull_request_number"] = pullRequestNumber
	}
	if deleted := viper.GetBool("deleted"); deleted {
		params["deleted"] = "true"
	}
	if page := viper.GetInt("page"); page != 0 {
		params["page"] = strconv.Itoa(page)
	}
	if pageSize := viper.GetInt("page-size"); pageSize != 0 {
		params["page_size"] = strconv.Itoa(pageSize)
	}
	if org := c.OrgLookupFn(); org != "" {
		params["org"] = org
	}

	body, err := c.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "environment", "", "", params), "application/json", nil)
	if err != nil {
		return err
	}

	if viper.GetBool("json") {
		display.Println(body)
		return nil
	}

	r, err := types.UnmarshalManyEnvs(body)
	if err != nil {
		return err
	}
	if len(r.Data) == 0 {
		display.Println("No environments found in the org.")
		return nil
	}

	var data [][]string
	for i := range r.Data {
		i := i
		data = append(data, display.FormattedEnvironment(&r.Data[i])...)
	}
	columns := []string{"App", "UUID", "Ready", "Repo", "PR#", "URL"}
	display.RenderTable(os.Stdout, columns, data)
	if r.Links.Next != "" {
		display.Println(fmt.Sprintf("Table is truncated, fetch the next page %d.", r.Links.NextPage()))
	}
	return nil
}

func handleGetEnvironmentByID(c client.Client, id string) error {
	params := make(map[string]string)
	if org := c.OrgLookupFn(); org != "" {
		params["org"] = org
	}

	body, err := c.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id, "", params), "application/json", nil)
	if err != nil {
		return err
	}

	if viper.GetBool("json") {
		display.Println(body)
		return nil
	}

	r, err := types.UnmarshalEnv(body)
	if err != nil {
		return err
	}

	data := display.FormattedEnvironment(&r.Data)
	columns := []string{"App", "UUID", "Ready", "Repo", "PR#", "URL"}
	display.RenderTable(os.Stdout, columns, data)
	return nil
}
