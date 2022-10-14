package cmd

import (
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"shipyard/cmd/env"
)

var rootCmd = &cobra.Command{
	Use:     "shipyard",
	Short:   "The Shipyard CLI",
	Long:    `A tool to manage Ephemeral Environments on the Shipyard platform`,
	Version: "0.0.1",
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

	setupCommands()
}

func setupLogging(w io.Writer, prefix string) {
	log.SetOutput(w)
	log.SetPrefix(prefix)
}

func setupCommands() {
	rootCmd.AddCommand(NewGetCmd())
	rootCmd.AddCommand(NewSetCmd())

	rootCmd.AddCommand(env.NewCancelCmd())
	rootCmd.AddCommand(env.NewRebuildCmd())
	rootCmd.AddCommand(env.NewRestartCmd())
	rootCmd.AddCommand(env.NewReviveCmd())
	rootCmd.AddCommand(env.NewStopCmd())

}
