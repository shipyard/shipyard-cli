package org

import (
	"errors"
	"net/http"
	"strings"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/types"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewGetAllOrgsCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "orgs",
		Aliases: []string{"organizations"},
		Short:   "Get all orgs",
		Long: `Lists all orgs, to which the user belongs.
Note that this command requires a user-level access token.`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("json", cmd.Flags().Lookup("json"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return getAllOrgs(c)
		},
	}

	cmd.Flags().Bool("json", false, "JSON output")

	return cmd
}

func NewGetCurrentOrgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "org",
		Aliases:      []string{"organization"},
		Short:        "Get the currently configured org",
		Long:         "Gets the org that is currently set in the default or custom config",
		Example:      `  shipyard get org`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getCurrentOrg()
		},
	}

	return cmd
}

func getCurrentOrg() error {
	org := viper.GetString("org")
	if org == "" {
		return errors.New("no org is found in the config")
	}
	display.Println(org)
	return nil
}

func getAllOrgs(c client.Client) error {
	body, err := c.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "org", "", "", nil), "application/json", nil)
	if err != nil {
		return err
	}

	if viper.GetBool("json") {
		display.Println(body)
		return nil
	}

	orgs, err := types.UnmarshalOrgs(body)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(orgs.Data))
	for _, item := range orgs.Data {
		names = append(names, item.Attributes.Name)
	}

	display.Println(strings.Join(names, "\n"))
	return nil
}
