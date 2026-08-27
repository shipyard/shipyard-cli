package env

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/spf13/cobra"
)

// validBuildOnCommit are the accepted values for the --build-on-commit flag and the
// values of the --build-on-commit-for map, mirroring the external API.
var validBuildOnCommit = map[string]bool{"always": true, "inherit": true, "never": true}

// detachedDeployResponse models the JSON returned by the external
// POST /api/v1/application-build/<uuid>/detached-app-build endpoint.
type detachedDeployResponse struct {
	Data struct {
		Message              string `json:"message"`
		ApplicationUUID      string `json:"application_uuid"`
		ApplicationBuildUUID string `json:"application_build_uuid"`
		DisplayName          string `json:"display_name"`
	} `json:"data"`
}

func NewDetachedCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "detached",
		GroupID: constants.GroupEnvironments,
		Short:   "Manage detached environments",
		Long:    `Create and manage detached environments, which are independent clones of an existing environment.`,
	}

	cmd.AddCommand(newDetachedDeployCmd(c))

	return cmd
}

func newDetachedDeployCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [application build ID]",
		Short: "Deploy a detached environment from a source application build",
		Long: `Create a new, independent ("detached") environment by cloning an existing application build.

The new environment copies the source environment's configuration (secrets, build args,
env vars) and then runs on its own, with no link back to the source.`,
		Example: `  # Deploy a detached environment from application build 1a2b3c
  shipyard detached deploy 1a2b3c --name pr-preview

  # Override branches for specific repos and never rebuild on new commits
  shipyard detached deploy 1a2b3c --name pr-preview --branch web=feature-x --build-on-commit never

  # Per-repo build-on-commit settings
  shipyard detached deploy 1a2b3c --build-on-commit-for web=always --build-on-commit-for api=never`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deployDetached(cmd, c, args[0])
		},
	}

	cmd.Flags().String("name", "", "Display name for the new detached environment")
	cmd.Flags().StringToString("branch", nil, "Per-repo branch override, as repo=branch (repeatable)")
	cmd.Flags().String("build-on-commit", "", "Rebuild on new commits for all repos: always, inherit, or never (default never)")
	cmd.Flags().StringToString("build-on-commit-for", nil, "Per-repo build-on-commit setting, as repo=setting (repeatable)")

	return cmd
}

func deployDetached(cmd *cobra.Command, c client.Client, appBuildID string) error {
	name, _ := cmd.Flags().GetString("name")
	branches, _ := cmd.Flags().GetStringToString("branch")
	buildOnCommit, _ := cmd.Flags().GetString("build-on-commit")
	buildOnCommitFor, _ := cmd.Flags().GetStringToString("build-on-commit-for")

	// build_on_commit can be a global string or a per-repo object, but not both.
	if buildOnCommit != "" && len(buildOnCommitFor) > 0 {
		return fmt.Errorf("--build-on-commit and --build-on-commit-for are mutually exclusive")
	}
	if buildOnCommit != "" && !validBuildOnCommit[buildOnCommit] {
		return fmt.Errorf("invalid --build-on-commit %q: must be always, inherit, or never", buildOnCommit)
	}
	for repo, setting := range buildOnCommitFor {
		if !validBuildOnCommit[setting] {
			return fmt.Errorf("invalid --build-on-commit-for %s=%s: setting must be always, inherit, or never", repo, setting)
		}
	}

	// Assemble the request body, omitting empty fields so the API applies its defaults.
	payload := make(map[string]any)
	if name != "" {
		payload["display_name"] = name
	}
	if len(branches) > 0 {
		payload["project_branch_overrides"] = branches
	}
	switch {
	case len(buildOnCommitFor) > 0:
		payload["build_on_commit"] = buildOnCommitFor
	case buildOnCommit != "":
		payload["build_on_commit"] = buildOnCommit
	}

	params := make(map[string]string)
	if org := c.OrgLookupFn(); org != "" {
		params["org"] = org
	}

	body, err := c.Requester.Do(
		http.MethodPost,
		uri.CreateResourceURI("", "application-build", appBuildID, "detached-app-build", params),
		"application/json",
		payload,
	)
	if err != nil {
		return err
	}

	var resp detachedDeployResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("could not parse response: %w", err)
	}

	display.Println(fmt.Sprintf(
		"Detached environment %q deployed (application UUID: %s).",
		resp.Data.DisplayName, resp.Data.ApplicationUUID,
	))
	return nil
}
