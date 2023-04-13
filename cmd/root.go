package cmd

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/util/homedir"

	"github.com/shipyard/shipyard-cli/cmd/env"
	"github.com/shipyard/shipyard-cli/cmd/k8s"
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
		logging.Init()
		log.Println("Git commit:", version.GitCommit)
		log.Println("Current config file:", viper.ConfigFileUsed())
	},
}

var (
	cfgFile        string
	red            = color.New(color.FgHiRed)
	errConfigParse = errors.New("failed to parse the config file, check YAML for syntax errors")
)

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		_, _ = red.Fprintln(os.Stderr, "Error:", err.Error())
	}
}

func init() {
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetEnvPrefix("shipyard")
	viper.AutomaticEnv()
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.shipyard/config.yaml)")

	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	rootCmd.PersistentFlags().String("org", "", "Org of environment (default org if unspecified)")
	_ = viper.BindPFlag("org", rootCmd.PersistentFlags().Lookup("org"))

	setupCommands()
}

func setupCommands() {
	rootCmd.AddCommand(NewGetCmd())
	rootCmd.AddCommand(NewSetCmd())

	rootCmd.AddGroup(&cobra.Group{ID: constants.GroupEnvironments, Title: "Environments"})
	rootCmd.AddCommand(env.NewCancelCmd())
	rootCmd.AddCommand(env.NewRebuildCmd())
	rootCmd.AddCommand(env.NewRestartCmd())
	rootCmd.AddCommand(env.NewReviveCmd())
	rootCmd.AddCommand(env.NewStopCmd())
	rootCmd.AddCommand(env.NewVisitCmd())

	rootCmd.AddCommand(k8s.NewExecCmd())
	rootCmd.AddCommand(k8s.NewLogsCmd())
	rootCmd.AddCommand(k8s.NewPortForwardCmd())
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			if errors.As(err, &viper.ConfigParseError{}) {
				initFail(errConfigParse)
			}
			initFail(err)
		}
		return
	}

	home := homedir.HomeDir()
	if home == "" {
		initFail(errors.New("home directory not found"))
	}

	viper.AddConfigPath(filepath.Join(home, ".shipyard"))
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			// Create an empty config for the user.
			if err := config.CreateDefaultConfig(home); err != nil {
				initFail(err)
			}
			_, _ = fmt.Fprintln(os.Stdout, "Creating a default config.yaml in $HOME/.shipyard")
			return
		} else if errors.As(err, &viper.ConfigParseError{}) {
			initFail(errConfigParse)
		} else {
			initFail(err)
		}
	}
}

func initFail(err error) {
	_, _ = red.Fprintf(os.Stderr, "Init error: %s\n", err)
	os.Exit(1)
}
