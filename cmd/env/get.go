package env

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"shipyard/requests"
	"shipyard/requests/uri"
)

func NewGetEnvironmentCmd() *cobra.Command {
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

	cmd.Flags().Bool("json", false, "JSON output")
	viper.BindPFlag("json", cmd.Flags().Lookup("json"))

	return cmd
}

func NewGetAllEnvironmentsCmd() *cobra.Command {
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

	cmd.Flags().Bool("json", false, "JSON output")
	viper.BindPFlag("json", cmd.Flags().Lookup("json"))

	return cmd
}

func getAllEnvironments() error {
	client, err := requests.NewHTTPClient(os.Stdout)
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

	if viper.GetBool("json") {
		return client.Write(body)
	}

	r, err := unmarshalManyEnv(body)
	if err != nil {
		return err
	}

	var data [][]string
	for _, d := range r.Data {
		data = append(data, []string{
			d.ID,
			d.Attributes.Projects[0].RepoName,
			d.Attributes.Name,
			strconv.Itoa(d.Attributes.Projects[0].PullRequestNumber),
			d.Attributes.URL,
		})
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"UUID", "Repo", "AppName", "PR#", "URL"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")

	for _, v := range data {
		table.Append(v)
	}
	table.Render()

	return nil
}

func getEnvironmentByID(id string) error {
	client, err := requests.NewHTTPClient(os.Stdout)
	if err != nil {
		return err
	}

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id, nil), nil)
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

	env := r.Data
	data := [][]string{
		[]string{
			env.ID,
			env.Attributes.Projects[0].RepoName,
			env.Attributes.Name,
			strconv.Itoa(env.Attributes.Projects[0].PullRequestNumber),
			env.Attributes.URL,
		},
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"UUID", "Repo", "AppName", "PR#", "URL"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")

	for _, v := range data {
		table.Append(v)
	}
	table.Render()

	return nil
}

func unmarshalEnv(p []byte) (respOneEnv, error) {
	var r respOneEnv
	err := json.Unmarshal(p, &r)
	return r, err
}

func unmarshalManyEnv(p []byte) (respManyEnvs, error) {
	var r respManyEnvs
	err := json.Unmarshal(p, &r)
	return r, err
}

type respOneEnv struct {
	Data struct {
		Attributes struct {
			Name     string `json:"name"`
			URL      string `json:"url"`
			Projects []struct {
				PullRequestNumber int    `json:"pull_request_number"`
				RepoName          string `json:"repo_name"`
			} `json:"projects"`
		} `json:"attributes"`

		ID string `json:"id"`
	} `json:"data"`
}

type respManyEnvs struct {
	Data []struct {
		Attributes struct {
			Name     string `json:"name"`
			URL      string `json:"url"`
			Projects []struct {
				PullRequestNumber int    `json:"pull_request_number"`
				RepoName          string `json:"repo_name"`
			} `json:"projects"`
		} `json:"attributes"`

		ID string `json:"id"`
	} `json:"data"`
}
