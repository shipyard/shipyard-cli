package org

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/types"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewGetAllOrgsCmd() *cobra.Command {
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
			return getAllOrgs()
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
	d := display.New()
	org := viper.GetString("org")
	if org == "" {
		return errors.New("no org is found in the config")
	}
	d.Println(org)
	return nil
}

func getAllOrgs() error {
	requester, err := requests.New(os.Stdout)
	if err != nil {
		return err
	}

	body, err := requester.Do(http.MethodGet, uri.CreateResourceURI("", "org", "", "", nil), nil)
	if err != nil {
		return err
	}

	if viper.GetBool("json") {
		return requester.Write(body)
	}

	orgs, err := types.UnmarshalOrgs(body)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(orgs.Data))
	for _, item := range orgs.Data {
		names = append(names, item.Attributes.Name)
	}

	return requester.Write(strings.Join(names, "\n") + "\n")
}
