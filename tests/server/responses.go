package server

import "github.com/shipyard/shipyard-cli/pkg/types"

//nolint:gochecknoglobals // OK for testing.
var store = map[string][]types.Environment{
	"default": {
		{
			Attributes: types.EnvironmentAttributes{
				URL:   "https://dev.example.com",
				Ready: true,
				Projects: []types.Project{
					{PullRequestNumber: 123, RepoName: "Repo1"},
					{PullRequestNumber: 456, RepoName: "Repo2"},
				},
				Services: []types.Service{
					{
						Name: "postgres",
					},
					{
						Name: "web",
					},
				},
			},
			ID: "default-1",
		},
		{
			Attributes: types.EnvironmentAttributes{
				URL:   "https://dev.example.com",
				Ready: true,
				Projects: []types.Project{
					{PullRequestNumber: 123, RepoName: "Repo1"},
					{PullRequestNumber: 456, RepoName: "Repo2"},
				},
			},
			ID: "default-2",
		},
	},
	"pugs": {
		{
			Attributes: types.EnvironmentAttributes{
				URL:   "https://prod.example.com",
				Ready: true,
				Projects: []types.Project{
					{PullRequestNumber: 900, RepoName: "pugs"},
				},
				Services: []types.Service{
					{
						Name: "mysql",
					},
					{
						Name: "nginx",
					},
				},
			},
			ID: "pug-1",
		},
		{
			Attributes: types.EnvironmentAttributes{
				URL:   "https://prod.example.com",
				Ready: true,
				Projects: []types.Project{
					{PullRequestNumber: 901, RepoName: "pugs"},
				},
			},
			ID: "pug-2",
		},
	},
}
