package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/display"
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
	var profile string

	cmd := &cobra.Command{
		Use:          "token",
		Short:        "Set the API token in the config",
		Long:         `Set the API token in the config by providing an argument, or interactively by running the command without arguments`,
		SilenceUsage: true,
		Example:      `  shipyard set token <token>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return setTokenInteractively(os.Stdin, profile)
			}
			return SetToken(args[0], profile)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name to save token under (hidden)")
	_ = cmd.Flags().MarkHidden("profile")

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

func setTokenInteractively(r io.Reader, profile string) error {
	display.Print("Your API token: ")

	var token string
	_, err := fmt.Fscanln(r, &token)
	if err != nil {
		return err
	}

	return SetToken(token, profile)
}

func SetToken(token string, profile string) error {
	viper.Set("api_token", token)

	// If a profile is specified, save the token under that profile as well
	if profile != "" {
		profiles := viper.GetStringMap("profiles")
		if profiles == nil {
			profiles = make(map[string]interface{})
		}

		// Create or update the profile with the auth_token
		profileData := make(map[string]interface{})
		if existingProfile, exists := profiles[profile]; exists {
			if profileMap, ok := existingProfile.(map[string]interface{}); ok {
				profileData = profileMap
			}
		}
		profileData["auth_token"] = token
		profiles[profile] = profileData

		viper.Set("profiles", profiles)
	}

	// TODO: find a better way to not persist the value of verbose globally.
	viper.Set("verbose", false)
	err := viper.MergeInConfig()
	if err != nil {
		return err
	}
	return viper.WriteConfig()
}
