package k8s

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/k8s"
)

func NewLogsCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs",
		GroupID: constants.GroupEnvironments,
		Aliases: []string{"log"},
		Short:   "Get logs from a service in an environment",
		Example: `  # Get logs for service flask-backend:
  shipyard logs --env 12345 --service flask-backend

  # Follow logs for the flask-backend service:
  shipyard logs --env 12345 --service flask-backend --follow

  # Get last 100 lines of logs for the flask-backend service:
  shipyard logs --env 12345 --service flask-backend --tail 100`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("service", cmd.Flags().Lookup("service"))
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
			_ = viper.BindPFlag("follow", cmd.Flags().Lookup("follow"))
			_ = viper.BindPFlag("tail", cmd.Flags().Lookup("tail"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleLogsCmd(c)
		},
	}

	cmd.Flags().String("service", "", "Service name")
	_ = cmd.MarkFlagRequired("service")

	cmd.Flags().String("env", "", "Environment ID")
	_ = cmd.MarkFlagRequired("env")

	cmd.Flags().BoolP("follow", "f", false, "Follow the log output")
	cmd.Flags().Int64("tail", 3000, "Number of lines from the end of the logs to show")

	return cmd
}

func handleLogsCmd(c client.Client) error {
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

	follow := viper.GetBool("follow")
	tail := viper.GetInt64("tail")

	return k.Logs(follow, tail)
}
