package cmd

import (
	"os"

	"shipyard/cmd/cancel"
	"shipyard/cmd/get"
	"shipyard/cmd/rebuild"
	"shipyard/cmd/restart"
	"shipyard/cmd/revive"
	"shipyard/cmd/stop"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "shipyard",
	Short:   "The Shipyard CLI",
	Long:    `A tool to manage Ephemeral Environments on the Shipyard platform`,
	Version: "0.1",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.shipyard.yaml)")

	versionTemplate := `{{printf "%s: %s - version %s\n" .Name .Short .Version}}`
	rootCmd.SetVersionTemplate(versionTemplate)

	getCmd := get.NewGetCmd()
	cancelCmd := cancel.NewCancelCmd()
	reviveCmd := revive.NewReviveCmd()
	rebuildCmd := rebuild.NewRebuildCmd()
	stopCmd := stop.NewStopCmd()
	restartCmd := restart.NewRestartCmd()

	rootCmd.AddCommand(cancelCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(reviveCmd)
	rootCmd.AddCommand(rebuildCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)
}
