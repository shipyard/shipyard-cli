package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/display"
)

func NewSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set a value in the default local config",
	}

	cmd.AddCommand(NewSetOrgCmd())
	cmd.AddCommand(NewSetTokenCmd())

	return cmd
}

func NewSetOrgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "org",
		Aliases:      []string{"organization"},
		Short:        "Set the org in the config",
		SilenceUsage: true,
		Example:      `  shipyard set org myorg`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("org not provided")
			}
			return setOrg(args[0])
		},
	}

	return cmd
}

func NewSetTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "token",
		Short:        "Set the API token in the config",
		Long:         `Set the API token in the config by providing an argument, or interactively by running the command without arguments`,
		SilenceUsage: true,
		Example:      `  shipyard set token <token>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return setTokenInteractively(os.Stdin)
			}
			return setToken(args[0])
		},
	}

	return cmd
}

func setOrg(name string) error {
	viper.Set("org", name)
	err := viper.MergeInConfig()
	if err != nil {
		return err
	}
	return viper.WriteConfig()
}

func setTokenInteractively(r io.Reader) error {
	out := display.NewSimpleDisplay()
	out.Print("Your API token: ")

	var token string
	_, err := fmt.Fscanln(r, &token)
	if err != nil {
		return err
	}

	return setToken(token)
}

func setToken(token string) error {
	viper.Set("shipyard_api_token", token)
	err := viper.MergeInConfig()
	if err != nil {
		return err
	}
	return viper.WriteConfig()
}
