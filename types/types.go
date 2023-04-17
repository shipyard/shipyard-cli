package types

type Service struct {
	Name          string   `json:"name"`
	Ports         []string `json:"ports"`
	SanitizedName string   `json:"sanitized_name"`
	URL           string   `json:"url"`
}

type Environment struct {
	Attributes struct {
		Name  string `json:"name"`
		URL   string `json:"url"`
		Ready bool   `json:"ready"`

		Projects []struct {
			PullRequestNumber int    `json:"pull_request_number"`
			RepoName          string `json:"repo_name"`
		} `json:"projects"`

		Services []Service `json:"services"`
	} `json:"attributes"`

	ID string `json:"id"`
}
