package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewModeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "mode [local|qa|prod]",
		Short:  "Set Shipyard build URL mode",
		Hidden: true, // This makes the command hidden from help
		Args:   cobra.ExactArgs(1),
		RunE:   runMode,
	}

	return cmd
}

func runMode(cmd *cobra.Command, args []string) error {
	mode := args[0]

	var buildURL string
	switch mode {
	case "local":
		buildURL = "http://localhost:8080/api/v1"
	case "qa":
		buildURL = "https://qa.shipyard.build/api/v1"
	case "werzer":
		buildURL = "https://werzer.shipyard.build/api/v1"
	case "prod":
		buildURL = "https://shipyard.build/api/v1"
	default:
		return fmt.Errorf("invalid mode: %s. Must be one of: local, qa, prod", mode)
	}

	// Save the API URL to the config file
	viper.Set("api_url", buildURL)

	// Copy token from profile to auth_token field if profile exists
	profiles := viper.GetStringMap("profiles")
	profileFound := false
	if profileData, exists := profiles[mode]; exists {
		if profileMap, ok := profileData.(map[string]interface{}); ok {
			if token, hasToken := profileMap["auth_token"]; hasToken {
				if tokenStr, ok := token.(string); ok {
					viper.Set("api_token", tokenStr)
					profileFound = true
				}
			}
		}
	}

	err := viper.MergeInConfig()
	if err != nil {
		return fmt.Errorf("failed to merge config: %w", err)
	}

	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Mode set to %s.\n", mode)
	if !profileFound {
		fmt.Printf("Don't forget to update your auth token or set new profiles using `shipyard set token --profile %s`.\n", mode)
	}
	return nil
}
