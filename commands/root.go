package commands

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/shipyard/shipyard-cli/auth"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/util/homedir"

	"github.com/shipyard/shipyard-cli/commands/env"
	"github.com/shipyard/shipyard-cli/commands/k8s"
	"github.com/shipyard/shipyard-cli/config"
	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/logging"
	"github.com/shipyard/shipyard-cli/version"
)

var rootCmd = &cobra.Command{
	Use:           "shipyard",
	Short:         "the CLI",
	Long:          `A tool to manage Ephemeral Environments on the Shipyard platform`,
	Version:       fmt.Sprintf("%s (Git Commit %s)", version.Version, version.GitCommit),
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Register()
		log.Println("Git commit:", version.GitCommit)
		log.Println("Current config file:", viper.ConfigFileUsed())
	},
}

var (
	cfgFile        string
	errConfigParse = errors.New("failed to parse the config file, check YAML for syntax errors")
)

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fail("Command", err)
	}
}

func init() {
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetEnvPrefix("shipyard")
	viper.AutomaticEnv()

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.shipyard/config.yaml)")

	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	rootCmd.PersistentFlags().String("org", "", "Org of environment (default org if unspecified)")
	_ = viper.BindPFlag("org", rootCmd.PersistentFlags().Lookup("org"))

	initConfig()
	setupCommands()
}

func setupCommands() {
	token, err := auth.GetAPIToken()
	if err != nil {
		warn("Token", err)
	}

	requester := requests.New(token)
	c := client.New(requester, viper.GetString("org"))
	rootCmd.AddCommand(NewGetCmd(c))
	rootCmd.AddCommand(NewSetCmd())

	rootCmd.AddGroup(&cobra.Group{ID: constants.GroupEnvironments, Title: "Environments"})
	rootCmd.AddCommand(env.NewCancelCmd(c))
	rootCmd.AddCommand(env.NewRebuildCmd(c))
	rootCmd.AddCommand(env.NewRestartCmd(c))
	rootCmd.AddCommand(env.NewReviveCmd(c))
	rootCmd.AddCommand(env.NewStopCmd(c))
	rootCmd.AddCommand(env.NewVisitCmd(c))

	rootCmd.AddCommand(k8s.NewExecCmd(c))
	rootCmd.AddCommand(k8s.NewLogsCmd(c))
	rootCmd.AddCommand(k8s.NewPortForwardCmd(c))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			if errors.As(err, &viper.ConfigParseError{}) {
				fail("Init", errConfigParse)
			}
			fail("Init", err)
		}
		return
	}

	home := homedir.HomeDir()
	if home == "" {
		fail("Init", errors.New("home directory not found"))
	}

	viper.AddConfigPath(filepath.Join(home, ".shipyard"))
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			// Create an empty config for the user.
			if err := config.CreateDefaultConfig(home); err != nil {
				fail("Init", err)
			}
			_, _ = fmt.Fprintln(os.Stdout, "Creating a default config.yaml in $HOME/.shipyard")
			return
		} else if errors.As(err, &viper.ConfigParseError{}) {
			fail("Init", errConfigParse)
		} else {
			fail("Init", err)
		}
	}
}

func fail(kind string, err error) {
	red := color.New(color.FgHiRed)
	_, _ = red.Fprintf(os.Stderr, fmt.Sprintf("%s error: %s\n", kind, err))
	os.Exit(1)
}

func warn(kind string, err error) {
	yellow := color.New(color.FgHiYellow)
	_, _ = yellow.Fprintf(os.Stderr, fmt.Sprintf("%s error: %s\n", kind, err))
}
