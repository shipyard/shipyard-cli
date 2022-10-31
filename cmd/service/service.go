package service

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"shipyard/cmd/env"
	"shipyard/requests"
)

func NewGetServicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "services",
		Short: "Get services in an environment",
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

	return client.Write(environment.Data.Attributes.Services)
}
