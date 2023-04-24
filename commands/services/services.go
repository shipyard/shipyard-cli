package services

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/commands/env"
	"github.com/shipyard/shipyard-cli/display"
	"github.com/shipyard/shipyard-cli/requests"
	"github.com/shipyard/shipyard-cli/types"
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

func GetAllByEnvironment(id string) ([]types.Service, error) {
	if id == "" {
		return nil, fmt.Errorf("environment ID is missing")
	}
	client, err := requests.NewClient(io.Discard)
	if err != nil {
		return nil, err
	}

	environment, err := env.GetEnvironmentByID(client, id)
	if err != nil {
		return nil, err
	}

	services := environment.Data.Attributes.Services
	if len(services) == 0 {
		return nil, fmt.Errorf("no services found for environment, check if it's running")
	}
	return services, nil
}

func handleGetServicesCmd() error {
	id := viper.GetString("env")
	services, err := GetAllByEnvironment(id)
	if err != nil {
		return fmt.Errorf("failed to get services for environment %s: %w", id, err)
	}

	var data [][]string
	for _, s := range services {
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
