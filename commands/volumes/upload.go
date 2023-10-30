package volumes

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

func NewUploadCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use: "upload",
	}
	cmd.AddCommand(NewUploadVolumeCmd(c))
	return cmd
}

func NewUploadVolumeCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "volume",
		Short:        "Upload a file to a volume in an environment",
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
			_ = viper.BindPFlag("volume", cmd.Flags().Lookup("volume"))
			_ = viper.BindPFlag("file", cmd.Flags().Lookup("file"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleUploadVolumeCmd(c)
		},
	}

	cmd.Flags().String("env", "", "environment ID")
	cmd.Flags().String("volume", "", "volume name")
	cmd.Flags().String("file", "", "file to upload")
	_ = cmd.MarkFlagRequired("env")
	_ = cmd.MarkFlagRequired("volume")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func handleUploadVolumeCmd(c client.Client) error {
	envID := viper.GetString("env")
	volume := viper.GetString("volume")
	params := make(map[string]string)
	if c.Org != "" {
		params["Org"] = c.Org
	}

	subresource := fmt.Sprintf("volume/%s/upload", volume)
	url := uri.CreateResourceURI("", "environment", envID, subresource, params)
	form, contentType, err := fileForm(viper.GetString("file"), "volume_tarball")
	if err != nil {
		return err
	}
	_, err = c.Requester.Do(http.MethodPost, url, contentType, form)
	return err
}

func fileForm(path, formField string) (*bytes.Buffer, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	var bodyBuf bytes.Buffer
	bodyWriter := multipart.NewWriter(&bodyBuf)
	defer bodyWriter.Close()

	fileWriter, err := bodyWriter.CreateFormFile(formField, path)
	if err != nil {
		return nil, "", err
	}
	if _, err = io.Copy(fileWriter, file); err != nil {
		return nil, "", err
	}
	return &bodyBuf, bodyWriter.FormDataContentType(), nil
}
