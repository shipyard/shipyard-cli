package types

type Service struct {
	Name          string   `json:"name"`
	Ports         []string `json:"ports"`
	SanitizedName string   `json:"sanitized_name"`
	URL           string   `json:"url"`
}

type Environment struct {
	ID         string                `json:"id"`
	Attributes EnvironmentAttributes `json:"attributes"`
}

type Project struct {
	PullRequestNumber int    `json:"pull_request_number"`
	RepoName          string `json:"repo_name"`
}

type EnvironmentAttributes struct {
	Name     string    `json:"name"`
	URL      string    `json:"url"`
	Ready    bool      `json:"ready"`
	Projects []Project `json:"projects"`
	Services []Service `json:"services"`
}

type Volume struct {
	Attributes VolumeAttributes `json:"attributes"`
	ID         string           `json:"id"`
	Type       string           `json:"type"`
}

type VolumeAttributes struct {
	ComposePath      string `json:"compose_path"`
	RemoteComposeURL string `json:"remote_compose_url"`
	Name             string `json:"volume_name"`
	ServiceName      string `json:"service_name"`
	VolumePath       string `json:"volume_path"`
}

type Snapshot struct {
	Attributes SnapshotAttributes `json:"attributes"`
	ID         string             `json:"id"`
	Type       string             `json:"type"`
}

type SnapshotAttributes struct {
	CreatedAt          string `json:"created_at"`
	FromSnapshotNumber int    `json:"from_snapshot_number"`
	SequenceNumber     int    `json:"sequence_number"`
	Status             string `json:"status"`
	TotalSize          int    `json:"total_size"`
}
