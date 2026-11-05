package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/config"
)

// modeProfile is the mode a command is running under, set while mode runs it.
var modeProfile string

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
  shipyard mode local get environments

  # Flags after the mode go to the command
  shipyard mode qa get environment 12345 --bypass-token`,
		Args: cobra.MinimumNArgs(1),
		RunE: runMode,
	}

	// Flags after the mode belong to the command it runs, which mode doesn't know about
	cmd.Flags().SetInterspersed(false)

	return cmd
}

func runMode(cmd *cobra.Command, args []string) error {
	mode := args[0]

	// Get the build URL for the mode
	buildURL, err := getBuildURLForMode(mode)
	if err != nil {
		return err
	}

	subArgs := args[1:]
	if len(subArgs) > 0 && subArgs[0] == "--" {
		subArgs = subArgs[1:]
	}

	// Without a command, set mode permanently (legacy behavior)
	root := cmd.Root()
	if target, _, err := root.Find(subArgs); err == nil && target == root {
		if err := root.ParseFlags(subArgs); errors.Is(err, pflag.ErrHelp) {
			return cmd.Help()
		} else if err != nil {
			return err
		}
		// Find stops at a separator, so the command can still be after one
		subArgs = root.Flags().Args()
		if len(subArgs) == 0 {
			loadConfig()
			logStartup()
			return setPermanentMode(mode, buildURL)
		}
	}

	// Otherwise, execute the subcommand with temporary mode context
	return executeWithMode(cmd, mode, buildURL, subArgs)
}

// profileTokenKey is where the config keeps a mode's profile token.
func profileTokenKey(mode string) string {
	return "profiles." + mode + ".auth_token"
}

// applyModeProfile overrides the token with the one the mode's profile sets, if
// it sets one. An empty one masks a token inherited from the environment.
func applyModeProfile() {
	if key := profileTokenKey(modeProfile); viper.IsSet(key) {
		os.Setenv("SHIPYARD_API_TOKEN", viper.GetString(key))
	}
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
	values := map[string]any{"api_url": buildURL}

	// Use the token the profile sets as api_token. An empty one clears it, or the
	// saved token, which belongs to the environment being left, goes to this one.
	key := profileTokenKey(mode)
	tokenSet := viper.IsSet(key)
	if tokenSet {
		values["api_token"] = viper.GetString(key)
	}

	if err := config.Save(values); err != nil {
		return err
	}

	fmt.Printf("Mode set to %s.\n", mode)
	if !tokenSet {
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

	// initConfig sets the token from the profile, in the run below: only that
	// run's flags say which config to read
	modeProfile = mode

	// Restore original environment variables when done
	defer func() {
		modeProfile = ""
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
