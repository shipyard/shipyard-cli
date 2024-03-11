package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/util/homedir"

	"github.com/shipyard/shipyard-cli/commands"
	"github.com/shipyard/shipyard-cli/commands/env"
	"github.com/shipyard/shipyard-cli/commands/k8s"
	"github.com/shipyard/shipyard-cli/commands/telepresence"
	"github.com/shipyard/shipyard-cli/config"
	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/logging"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests"
	"github.com/shipyard/shipyard-cli/version"
)

func main() {
	var (
		cfgFile string
	)

	r := &cobra.Command{
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

	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetEnvPrefix("shipyard")
	viper.AutomaticEnv()

	r.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.shipyard/config.yaml)")

	r.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	_ = viper.BindPFlag("verbose", r.PersistentFlags().Lookup("verbose"))

	r.PersistentFlags().String("org", "", "Org of environment (default org if unspecified)")
	_ = viper.BindPFlag("org", r.PersistentFlags().Lookup("org"))

	// Handle a panic.
	defer func() {
		if err := recover(); err != nil {
			red := color.New(color.FgHiRed)
			_, _ = red.Fprintf(os.Stderr, "Runtime error: %v\n", err)
			_, _ = fmt.Fprintln(os.Stderr, string(debug.Stack()))
			os.Exit(1)
		}
	}()

	initConfig(cfgFile)

	setupCommands(r)

	if err := r.Execute(); err != nil {
		fail("Run", err)
	}
}

func setupCommands(r *cobra.Command) {
	requester := requests.New()
	orgLookupFn := func() string {
		return viper.GetString("org")
	}
	c := client.New(requester, orgLookupFn)
	r.AddCommand(commands.NewLoginCmd())
	r.AddCommand(commands.NewGetCmd(c))
	r.AddCommand(commands.NewSetCmd())

	r.AddGroup(&cobra.Group{ID: constants.GroupEnvironments, Title: "Environments"})
	r.AddCommand(env.NewCancelCmd(c))
	r.AddCommand(env.NewRebuildCmd(c))
	r.AddCommand(env.NewRestartCmd(c))
	r.AddCommand(env.NewReviveCmd(c))
	r.AddCommand(env.NewStopCmd(c))
	r.AddCommand(env.NewVisitCmd(c))

	r.AddCommand(k8s.NewExecCmd(c))
	r.AddCommand(k8s.NewLogsCmd(c))
	r.AddCommand(k8s.NewPortForwardCmd(c))

	r.AddCommand(telepresence.NewConnectCmd(c))
}

func initConfig(cfgFile string) {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			if errors.As(err, &viper.ConfigParseError{}) {
				fail("Init", fmt.Errorf("failed to parse the config file, check YAML for syntax errors"))
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
		switch {
		case errors.As(err, &viper.ConfigFileNotFoundError{}):
			if err := config.CreateDefaultConfig(home); err != nil {
				fail("Init", err)
			}
			_, _ = fmt.Fprintln(os.Stdout, "Creating a default config.yaml in $HOME/.shipyard")
			return
		case errors.As(err, &viper.ConfigParseError{}):
			fail("Init", fmt.Errorf("failed to parse the config file, check YAML for syntax errors"))
		default:
			fail("Init", err)
		}
	}
}

func fail(kind string, err error) {
	red := color.New(color.FgHiRed)
	_, _ = red.Fprintf(os.Stderr, fmt.Sprintf("%s error: %s\n", kind, err))
	os.Exit(1)
}
