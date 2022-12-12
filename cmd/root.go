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
		log.Println("Using config file:", viper.ConfigFileUsed())
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		red := color.New(color.FgHiRed)
		red.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
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

var cfgFile string

func initConfig() {
	viper.AutomaticEnv()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			handleConfigParseError(err)
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	home := homedir.HomeDir()
	if home == "" {
		fmt.Fprintln(os.Stderr, "Home directory not found.")
		os.Exit(1)
	}

	viper.AddConfigPath(filepath.Join(home, ".shipyard"))
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			// Create an empty config for the user.
			p := filepath.Join(home, ".shipyard", "config.yaml")
			if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create the .shipyard directory in $HOME: %v\n", err)
				os.Exit(1)
			}
			if _, err = os.Create(p); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create the default config.yaml file in $HOME/.shipyard: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintln(os.Stdout, "Creating a default config.yaml in $HOME/.shipyard")
			return
		}
		handleConfigParseError(err)

		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func handleConfigParseError(err error) {
	if errors.As(err, &viper.ConfigParseError{}) {
		fmt.Fprintln(os.Stderr, "Failed to parse the config.yaml file, check YAML syntax for errors.")
		os.Exit(1)
	}
}
