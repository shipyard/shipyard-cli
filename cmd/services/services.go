package services

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"shipyard/cmd/env"
	"shipyard/display"
	"shipyard/requests"
)

func NewGetServicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "services",
		Short: "Get services in an environment",
		Example: `  # Get all services for environment ID 12345
  shipyard get services --env 12345`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("env", cmd.Flags().Lookup("env"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleGetServicesCmd()
		},
	}

	cmd.Flags().String("env", "", "environment ID")
	cmd.MarkFlagRequired("env")

	return cmd
}

func handleGetServicesCmd() error {
	client, err := requests.NewHTTPClient(os.Stdout)
	if err != nil {
		return err
	}

	environment, err := env.GetEnvironmentByID(client, viper.GetString("env"))
	if err != nil {
		return err
	}

	services := environment.Data.Attributes.Services
	if len(services) == 0 {
		return errors.New("no services found")
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
