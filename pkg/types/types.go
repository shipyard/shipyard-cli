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

type Volume struct {
	Attributes struct {
		ComposePath      string `json:"compose_path"`
		RemoteComposeURL string `json:"remote_compose_url"`
		ServiceName      string `json:"service_name"`
		VolumePath       string `json:"volume_path"`
	} `json:"attributes"`
	ID   string `json:"id"`
	Type string `json:"type"`
}

type Snapshot struct {
	Attributes struct {
		CreatedAt          string `json:"created_at"`
		FromSnapshotNumber int    `json:"from_snapshot_number"`
		SequenceNumber     int    `json:"sequence_number"`
		Status             string `json:"status"`
		TotalSize          int    `json:"total_size"`
	} `json:"attributes"`
	ID   string `json:"id"`
	Type string `json:"type"`
}
