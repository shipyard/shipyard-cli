package cmd

import (
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"shipyard/cmd/cancel"
	"shipyard/cmd/get"
	"shipyard/cmd/rebuild"
	"shipyard/cmd/restart"
	"shipyard/cmd/revive"
	"shipyard/cmd/stop"
)

var rootCmd = &cobra.Command{
	Use:     "shipyard",
	Short:   "The Shipyard CLI",
	Long:    `A tool to manage Ephemeral Environments on the Shipyard platform`,
	Version: "0.1",
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	setupLogging(os.Stderr, "CLI ")

	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	versionTemplate := `{{printf "%s: %s - version %s\n" .Name .Short .Version}}`
	rootCmd.SetVersionTemplate(versionTemplate)

	rootCmd.AddCommand(get.NewGetCmd())
	rootCmd.AddCommand(cancel.NewCancelCmd())
	rootCmd.AddCommand(revive.NewReviveCmd())
	rootCmd.AddCommand(rebuild.NewRebuildCmd())
	rootCmd.AddCommand(stop.NewStopCmd())
	rootCmd.AddCommand(restart.NewRestartCmd())
}

func setupLogging(w io.Writer, prefix string) {
	log.SetOutput(w)
	log.SetPrefix(prefix)
}
