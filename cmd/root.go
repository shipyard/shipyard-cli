package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"shipyard/cmd/env"
	"shipyard/cmd/k8s"
	"shipyard/constants"
	"shipyard/logging"
	"shipyard/version"
)

var rootCmd = &cobra.Command{
	Use:     "shipyard",
	Short:   "The Shipyard CLI",
	Long:    `A tool to manage Ephemeral Environments on the Shipyard platform`,
	Version: version.Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Init()
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
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
