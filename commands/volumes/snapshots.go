package volumes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/types"
)

func NewGetVolumeSnapshotsCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "snapshots",
		Short:        "Get volume snapshots in an environment",
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
			_ = viper.BindPFlag("page", cmd.Flags().Lookup("page"))
			_ = viper.BindPFlag("page-size", cmd.Flags().Lookup("page-size"))
			_ = viper.BindPFlag("json", cmd.Flags().Lookup("json"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleGetVolumeSnapshotsCmd(c)
		},
	}

	cmd.Flags().String("env", "", "environment ID")
	cmd.Flags().Int("page", 1, "Page number requested")
	cmd.Flags().Int("page-size", 20, "Page size requested")
	cmd.Flags().Bool("json", false, "JSON output")
	_ = cmd.MarkFlagRequired("env")
	return cmd
}

func handleGetVolumeSnapshotsCmd(c client.Client) error {
	params := make(map[string]string)
	if org := viper.GetString("org"); org != "" {
		params["org"] = org
	}
	if page := viper.GetInt("page"); page != 0 {
		params["page"] = strconv.Itoa(page)
	}
	if pageSize := viper.GetInt("page-size"); pageSize != 0 {
		params["page_size"] = strconv.Itoa(pageSize)
	}
	id := viper.GetString("env")
	body, err := c.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id, "volume-snapshots", params), "application/json", nil)
	if err != nil {
		return err
	}
	if viper.GetBool("json") {
		display.Println(body)
		return nil
	}

	var resp types.SnapshotsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal snapshots: %w", err)
	}
	if len(resp.Data) == 0 {
		display.Println("No snapshots found for this environment.")
		return nil
	}

	var data [][]string
	for _, v := range resp.Data {
		data = append(data, []string{
			strconv.Itoa(v.Attributes.FromSnapshotNumber),
			strconv.Itoa(v.Attributes.SequenceNumber),
			v.Attributes.Status,
			v.Type,
		})
	}

	columns := []string{"From", "Sequence", "Status", "Type"}
	display.RenderTable(os.Stdout, columns, data)
	if resp.Links.Next != "" {
		display.Println(fmt.Sprintf("Table is truncated, fetch the next page %d.", resp.Links.NextPage()))
	}
	return nil
}

func NewLoadCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use: "load",
	}
	cmd.AddCommand(NewLoadVolumeSnapshotCmd(c))
	return cmd
}

func NewLoadVolumeSnapshotCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "snapshot",
		Short:        "Load volume snapshot in an environment",
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
			_ = viper.BindPFlag("sequence-number", cmd.Flags().Lookup("sequence-number"))
			_ = viper.BindPFlag("source-application-id", cmd.Flags().Lookup("source-application-id"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleLoadVolumeSnapshotCmd(c)
		},
	}

	cmd.Flags().String("env", "", "environment ID")
	cmd.Flags().String("sequence-number", "", "sequence number of a snapshot")
	cmd.Flags().String("source-application-id", "", "source application ID")
	_ = cmd.MarkFlagRequired("env")
	_ = cmd.MarkFlagRequired("sequence-number")
	return cmd
}

func handleLoadVolumeSnapshotCmd(c client.Client) error {
	params := make(map[string]string)
	if org := viper.GetString("org"); org != "" {
		params["org"] = org
	}
	id := viper.GetString("env")
	data := map[string]any{
		"data": map[string]any{
			"type": "snapshot-load",
			"attributes": map[string]any{
				"sequence_number":       viper.GetInt("sequence-number"),
				"source_application_id": viper.GetString("source-application-id"),
			},
		},
	}
	_, err := c.Requester.Do(http.MethodPost, uri.CreateResourceURI("", "environment", id, "snapshot-load", params), "application/json", data)
	return err
}
