package services

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/client/services"
	"github.com/shipyard/shipyard-cli/pkg/display"
)

func NewGetServicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "services",
		Short: "Get services in an environment",
		Example: `  # Get all services for environment ID 12345
  shipyard get services --env 12345`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleGetServicesCmd()
		},
	}

	cmd.Flags().String("env", "", "environment ID")
	_ = cmd.MarkFlagRequired("env")

	return cmd
}

func handleGetServicesCmd() error {
	id := viper.GetString("env")
	org := viper.GetString("org")
	svcs, err := services.GetAll(id, org)
	if err != nil {
		return fmt.Errorf("failed to get services for environment %s: %w", id, err)
	}

	var data [][]string
	for _, s := range svcs {
		var ports string
		if len(s.Ports) > 0 {
			ports = fmt.Sprintf("%s", s.Ports)
		}

		data = append(data, []string{
			s.Name,
			ports,
			s.URL,
		})
	}

	columns := []string{"Name", "Ports", "URL"}
	display.RenderTable(os.Stdout, columns, data)
	return nil
}
