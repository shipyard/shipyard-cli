package cmd

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/util/homedir"

	"shipyard/cmd/env"
	"shipyard/cmd/k8s"
	"shipyard/config"
	"shipyard/constants"
	"shipyard/logging"
	"shipyard/version"
)

var rootCmd = &cobra.Command{
	Use:           "shipyard",
	Short:         "The Shipyard CLI",
	Long:          `A tool to manage Ephemeral Environments on the Shipyard platform`,
	Version:       version.Version,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Init()
		log.Println("Current config file:", viper.ConfigFileUsed())
	},
}

var (
	cfgFile string
	red     = color.New(color.FgHiRed)
)

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fail("Error: " + err.Error())
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.shipyard/config.yaml)")

	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	rootCmd.PersistentFlags().String("org", "", "Org of environment (default org if unspecified)")
	viper.BindPFlag("org", rootCmd.PersistentFlags().Lookup("org"))

	versionTemplate := `{{printf "%s: %s - version %s\n" .Name .Short .Version}}`
	rootCmd.SetVersionTemplate(versionTemplate)

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

	rootCmd.AddCommand(k8s.NewExecCmd())
	rootCmd.AddCommand(k8s.NewLogsCmd())
	rootCmd.AddCommand(k8s.NewPortForwardCmd())
}

func initConfig() {
	viper.AutomaticEnv()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			if errors.As(err, &viper.ConfigParseError{}) {
				fail("Failed to parse the config file, check YAML for syntax errors.")
			}
			fail(err.Error())
		}
		return
	}

	home := homedir.HomeDir()
	if home == "" {
		fail("Home directory not found.")
	}

	viper.AddConfigPath(filepath.Join(home, ".shipyard"))
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			// Create an empty config for the user.
			if err := config.CreateDefaultConfig(home); err != nil {
				fail(err.Error())
			}
			fmt.Fprintln(os.Stdout, "Creating a default config.yaml in $HOME/.shipyard")
			return
		} else if errors.As(err, &viper.ConfigParseError{}) {
			fail("Failed to parse the config file, check YAML for syntax errors.")
		} else {
			fail(err.Error())
		}
	}
}

func fail(message string) {
	red.Fprintln(os.Stderr, message)
	os.Exit(1)
}
