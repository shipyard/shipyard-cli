package k8s

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/k8s"
)

func NewPortForwardCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "port-forward",
		GroupID: constants.GroupEnvironments,
		Aliases: []string{"pf"},
		Short:   "Port-forward to a service in an environment",
		Example: `  # Get an environment's services and exposed ports:
  shipyard get services --env 12345

  # port-forward "web" service's port 80:
  shipyard port-forward --env 12345 --service web --ports 80:80`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("ports", cmd.Flags().Lookup("ports"))
			_ = viper.BindPFlag("service", cmd.Flags().Lookup("service"))
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handlePortForwardCmd(c)
		},
	}

	cmd.Flags().StringSlice("ports", nil, "Ports (for example, 3000:80)")
	_ = cmd.MarkFlagRequired("ports")

	cmd.Flags().String("service", "", "Service name")
	_ = cmd.MarkFlagRequired("service")

	cmd.Flags().String("env", "", "Environment ID")
	_ = cmd.MarkFlagRequired("env")

	return cmd
}

func handlePortForwardCmd(c client.Client) error {
	id := viper.GetString("env")
	serviceName := viper.GetString("service")
	ports := viper.GetStringSlice("ports")

	s, err := c.FindService(serviceName, id)
	if err != nil {
		return err
	}

	k, err := k8s.New(c, id, s)
	if err != nil {
		return err
	}

	return k.PortForward(ports)
}
