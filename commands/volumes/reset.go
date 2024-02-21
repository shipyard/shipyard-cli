package volumes

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

func NewResetCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use: "reset",
	}
	cmd.AddCommand(NewResetVolumeCmd(c))
	return cmd
}

func NewResetVolumeCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "volume",
		Short:        "Reset a volume in an environment",
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
			_ = viper.BindPFlag("volume", cmd.Flags().Lookup("volume"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleResetVolumeCmd(c)
		},
	}

	cmd.Flags().String("env", "", "environment ID")
	cmd.Flags().String("volume", "", "volume name")
	_ = cmd.MarkFlagRequired("env")
	_ = cmd.MarkFlagRequired("volume")

	return cmd
}

func handleResetVolumeCmd(c client.Client) error {
	envID := viper.GetString("env")
	volume := viper.GetString("volume")
	params := make(map[string]string)
	if org := c.OrgLookupFn(); org != "" {
		params["org"] = org
	}

	subresource := fmt.Sprintf("volume/%s/volume-reset", volume)
	_, err := c.Requester.Do(http.MethodPost, uri.CreateResourceURI("", "environment", envID, subresource, params), "application/json", nil)
	return err
}
