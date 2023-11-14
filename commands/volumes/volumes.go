package volumes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/types"
)

func NewGetVolumesCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "volumes",
		Short:        "Get volumes in an environment",
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
			_ = viper.BindPFlag("json", cmd.Flags().Lookup("json"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleGetVolumesCmd(c)
		},
	}

	cmd.Flags().String("env", "", "environment ID")
	cmd.Flags().Bool("json", false, "JSON output")
	_ = cmd.MarkFlagRequired("env")
	return cmd
}

func handleGetVolumesCmd(c client.Client) error {
	id := viper.GetString("env")
	params := make(map[string]string)
	if c.Org != "" {
		params["Org"] = c.Org
	}

	body, err := c.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id, "volumes", params), nil)
	if err != nil {
		return err
	}
	if viper.GetBool("json") {
		display.Println(body)
		return nil
	}

	var resp types.VolumesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal volumes: %w", err)
	}
	if len(resp.Data) == 0 {
		display.Println("No volumes found in the environment.")
		return nil
	}

	var data [][]string
	for _, v := range resp.Data {
		data = append(data, []string{
			v.Attributes.Name,
			v.Attributes.ServiceName,
		})
	}
	columns := []string{"Name", "Service"}
	display.RenderTable(os.Stdout, columns, data)
	return nil
}
