package env

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/display"
	"github.com/shipyard/shipyard-cli/requests"
	"github.com/shipyard/shipyard-cli/requests/uri"
)

var errNoEnvironment = errors.New("environment ID argument not provided")

func NewGetEnvironmentCmd() *cobra.Command {
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
			viper.BindPFlag("json", cmd.Flags().Lookup("json"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return handleGetEnvironmentByID(args[0])
			}
			return errNoEnvironment
		},
	}

	cmd.Flags().Bool("json", false, "JSON output")

	return cmd
}

func NewGetAllEnvironmentsCmd() *cobra.Command {
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
			viper.BindPFlag("name", cmd.Flags().Lookup("name"))
			viper.BindPFlag("org-name", cmd.Flags().Lookup("org-name"))
			viper.BindPFlag("repo-name", cmd.Flags().Lookup("repo-name"))
			viper.BindPFlag("branch", cmd.Flags().Lookup("branch"))
			viper.BindPFlag("pull-request-number", cmd.Flags().Lookup("pull-request-number"))
			viper.BindPFlag("deleted", cmd.Flags().Lookup("deleted"))
			viper.BindPFlag("page", cmd.Flags().Lookup("page"))
			viper.BindPFlag("page-size", cmd.Flags().Lookup("page-size"))
			viper.BindPFlag("json", cmd.Flags().Lookup("json"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleGetAllEnvironments()
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

var ErrUnmarshalling = errors.New("failed to unmarshal environment(s)")

// Converts the `environment` object to [][]string which is used during printing environments as table
func extractDataForTableOutput(env *environment) [][]string {
	var data [][]string

	for _, p := range env.Attributes.Projects {
		pr := strconv.Itoa(p.PullRequestNumber)
		if pr == "0" {
			pr = ""
		}

		data = append(data, []string{
			env.Attributes.Name,
			env.ID,
			fmt.Sprintf("%t", env.Attributes.Ready),
			p.RepoName,
			pr,
			env.Attributes.URL,
		})
	}

	return data
}

func handleGetAllEnvironments() error {
	client, err := requests.NewClient(os.Stdout)
	if err != nil {
		return err
	}

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
	if org := viper.GetString("org"); org != "" {
		params["org"] = org
	}

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "environment", "", "", params), nil)
	if err != nil {
		return err
	}

	if viper.GetBool("json") {
		return client.Write(body)
	}

	r, err := unmarshalManyEnvs(body)
	if err != nil {
		return ErrUnmarshalling
	}

	var data [][]string

	for _, d := range r.Data {
		data = append(data, extractDataForTableOutput(&d.environment)...)
	}

	columns := []string{"App", "UUID", "Ready", "Repo", "PR#", "URL"}
	display.RenderTable(os.Stdout, columns, data)

	return nil
}

// GetEnvironmentByID is a helper function that tries to fetch an environment,
// given a client and environment ID.
func GetEnvironmentByID(client requests.Client, id string) (*Response, error) {
	if id == "" {
		return nil, errors.New("environment ID is an empty string")
	}

	params := make(map[string]string)
	org := viper.GetString("org")
	if org != "" {
		params["org"] = org
	}

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id, "", params), nil)
	if err != nil {
		return nil, err
	}

	return unmarshalEnv(body)
}

func handleGetEnvironmentByID(id string) error {
	client, err := requests.NewClient(os.Stdout)
	if err != nil {
		return err
	}

	params := make(map[string]string)
	org := viper.GetString("org")
	if org != "" {
		params["org"] = org
	}

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id, "", params), nil)
	if err != nil {
		return err
	}

	if viper.GetBool("json") {
		return client.Write(body)
	}

	r, err := unmarshalEnv(body)
	if err != nil {
		return err
	}

	data := extractDataForTableOutput(&r.Data.environment)
	columns := []string{"App", "UUID", "Ready", "Repo", "PR#", "URL"}

	display.RenderTable(os.Stdout, columns, data)
	return nil
}

func unmarshalEnv(p []byte) (*Response, error) {
	var r Response
	err := json.Unmarshal(p, &r)
	if err != nil {
		return nil, ErrUnmarshalling
	}
	return &r, err
}

func unmarshalManyEnvs(p []byte) (*respManyEnvs, error) {
	var r respManyEnvs
	err := json.Unmarshal(p, &r)
	if err != nil {
		return nil, ErrUnmarshalling
	}
	return &r, err
}

type environment struct {
	Attributes struct {
		Name  string `json:"name"`
		URL   string `json:"url"`
		Ready bool   `json:"ready"`

		Projects []struct {
			PullRequestNumber int    `json:"pull_request_number"`
			RepoName          string `json:"repo_name"`
		} `json:"projects"`

		Services []struct {
			Name          string   `json:"name"`
			Ports         []string `json:"ports"`
			SanitizedName string   `json:"sanitized_name"`
			URL           string   `json:"url"`
		} `json:"services"`
	} `json:"attributes"`

	ID string `json:"id"`
}

type Response struct {
	Data struct {
		environment
	} `json:"data"`
}

type respManyEnvs struct {
	Data []struct {
		environment
	} `json:"data"`
}
