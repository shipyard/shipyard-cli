package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewModeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mode [local|qa|prod] [command...]",
		Short: "Set Shipyard build URL mode and optionally run a command",
		Long: `Set the Shipyard API endpoint mode and optionally execute a command with that context.

Examples:
  # Set mode permanently
  shipyard mode qa

  # Run MCP server in QA mode
  shipyard mode qa mcp serve -v

  # Run any command in local mode
  shipyard mode local get environments`,
		Args: cobra.MinimumNArgs(1),
		RunE: runMode,
	}

	return cmd
}

func runMode(cmd *cobra.Command, args []string) error {
	mode := args[0]

	// Get the build URL for the mode
	buildURL, err := getBuildURLForMode(mode)
	if err != nil {
		return err
	}

	// If no additional args, set mode permanently (legacy behavior)
	if len(args) == 1 {
		return setPermanentMode(mode, buildURL)
	}

	// Otherwise, execute the subcommand with temporary mode context
	return executeWithMode(cmd, mode, buildURL, args[1:])
}

func getBuildURLForMode(mode string) (string, error) {
	switch mode {
	case "local":
		return "http://localhost:8080/api/v1", nil
	case "qa":
		return "https://qa.shipyard.build/api/v1", nil
	case "werzer":
		return "https://werzer.shipyard.build/api/v1", nil
	case "prod":
		return "https://shipyard.build/api/v1", nil
	default:
		return "", fmt.Errorf("invalid mode: %s. Must be one of: local, qa, prod", mode)
	}
}

func setPermanentMode(mode, buildURL string) error {
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

func executeWithMode(parentCmd *cobra.Command, mode, buildURL string, subArgs []string) error {
	// Set environment variables to override config temporarily
	originalAPIURL := os.Getenv("SHIPYARD_API_URL")
	originalToken := os.Getenv("SHIPYARD_API_TOKEN")

	// Set the API URL for this execution
	os.Setenv("SHIPYARD_API_URL", buildURL)

	// Set token from profile if available
	profiles := viper.GetStringMap("profiles")
	if profileData, exists := profiles[mode]; exists {
		if profileMap, ok := profileData.(map[string]interface{}); ok {
			if token, hasToken := profileMap["auth_token"]; hasToken {
				if tokenStr, ok := token.(string); ok {
					os.Setenv("SHIPYARD_API_TOKEN", tokenStr)
				}
			}
		}
	}

	// Restore original environment variables when done
	defer func() {
		if originalAPIURL == "" {
			os.Unsetenv("SHIPYARD_API_URL")
		} else {
			os.Setenv("SHIPYARD_API_URL", originalAPIURL)
		}
		if originalToken == "" {
			os.Unsetenv("SHIPYARD_API_TOKEN")
		} else {
			os.Setenv("SHIPYARD_API_TOKEN", originalToken)
		}
	}()

	// Get the root command to execute the subcommand
	rootCmd := parentCmd.Root()

	// Set the args for the subcommand execution
	rootCmd.SetArgs(subArgs)

	// Execute the subcommand
	return rootCmd.Execute()
}
