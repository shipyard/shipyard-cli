package volumes

import (
	"net/http"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

func NewCreateCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use: "create",
	}
	cmd.AddCommand(NewCreateSnapshotCmd(c))
	return cmd
}

func NewCreateSnapshotCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "snapshot",
		Short:        "Create a snapshot in an environment",
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
			_ = viper.BindPFlag("note", cmd.Flags().Lookup("note"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleCreateSnapshotCmd(c)
		},
	}

	cmd.Flags().String("env", "", "environment ID")
	_ = cmd.MarkFlagRequired("env")
	cmd.Flags().String("note", "", "An optional description of the snapshot")

	return cmd
}

func handleCreateSnapshotCmd(c client.Client) error {
	envID := viper.GetString("env")
	params := make(map[string]string)
	if c.Org != "" {
		params["Org"] = c.Org
	}
	body := map[string]any{
		"note": viper.GetString("note"),
	}
	_, err := c.Requester.Do(http.MethodPost, uri.CreateResourceURI("", "environment", envID, "snapshot-create", params), "application/json", body)
	return err
}
