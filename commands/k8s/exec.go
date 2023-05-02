package k8s

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/k8s"
)

func NewExecCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "exec",
		GroupID: constants.GroupEnvironments,
		Short:   "Execute a command in a service in an environment",
		Long: `Execute any command with any arguments and flags in a given service.
You can also run interactive commands, like shells, without passing anything special to exec.

Pass any command arguments after a double slash.

shipyard exec --env 123 --service web -- ls -l -a
shipyard exec --env 123 --service web -- bash`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("service", cmd.Flags().Lookup("service"))
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleExecCmd(c, args)
		},
	}

	cmd.Flags().String("service", "", "Service name")
	_ = cmd.MarkFlagRequired("service")

	cmd.Flags().String("env", "", "Environment ID")
	_ = cmd.MarkFlagRequired("env")

	return cmd
}

func handleExecCmd(c client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("no command arguments provided")
	}

	serviceName := viper.GetString("service")
	id := viper.GetString("env")
	svc, err := c.FindService(serviceName, id)
	if err != nil {
		return err
	}

	k, err := k8s.New(c, id, svc)
	if err != nil {
		return err
	}

	return k.Exec(args)
}
