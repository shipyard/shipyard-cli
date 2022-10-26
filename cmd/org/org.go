package org

import (
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"shipyard/requests"
	"shipyard/requests/uri"
)

func NewGetOrgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Get current org",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	return cmd
}

func NewGetAllOrgsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orgs",
		Short: "Get all orgs",
		Long: `Lists all orgs, to which the user belongs.
Note that this command requires a user-level access token.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getAllOrgs()
		},
	}

	return cmd
}

func getAllOrgs() error {
	client, err := requests.NewHTTPClient(os.Stdout)
	if err != nil {
		return err
	}

	body, err := client.Do(http.MethodGet, uri.CreateResourceURI("", "org", "", nil), nil)
	if err != nil {
		return err
	}
	return client.Write(body)
}
