package volumes

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/zip"
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
			_ = viper.BindPFlag("path", cmd.Flags().Lookup("path"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleUploadVolumeCmd(c)
		},
	}

	cmd.Flags().String("env", "", "environment ID")
	cmd.Flags().String("volume", "", "volume name")
	cmd.Flags().String("path", "", "path to a file to upload (either a .bz2 archive, regular file, or directory")
	_ = cmd.MarkFlagRequired("env")
	_ = cmd.MarkFlagRequired("volume")
	_ = cmd.MarkFlagRequired("path")

	return cmd
}

func handleUploadVolumeCmd(c client.Client) error {
	envID := viper.GetString("env")
	volume := viper.GetString("volume")
	params := make(map[string]string)
	if org := c.OrgLookupFn(); org != "" {
		params["org"] = org
	}

	path := viper.GetString("path")
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	var archiveFilename string

	switch {
	case fi.IsDir():
		if err := zip.CreateArchiveFromDir(path); err != nil {
			return err
		}
		archiveFilename = path + ".tar.bz2"
	case !bz2File(path):
		if err := zip.CreateArchiveFromFile(path); err != nil {
			return err
		}
		archiveFilename = path + ".tar.bz2"
	default: // .bz2 file
		archiveFilename = path
	}

	file, err := os.Open(archiveFilename)
	if err != nil {
		return err
	}
	defer file.Close()

	subresource := fmt.Sprintf("volume/%s/upload", volume)
	url := uri.CreateResourceURI("", "environment", envID, subresource, params)
	form, contentType, err := fileForm(file, "volume_tarball")
	if err != nil {
		return err
	}
	_, err = c.Requester.Do(http.MethodPost, url, contentType, form)
	return err
}

func bz2File(path string) bool {
	return filepath.Ext(path) == ".bz2"
}

func fileForm(file *os.File, formField string) (*bytes.Buffer, string, error) {
	var bodyBuf bytes.Buffer
	bodyWriter := multipart.NewWriter(&bodyBuf)
	defer bodyWriter.Close()

	fileWriter, err := bodyWriter.CreateFormFile(formField, file.Name())
	if err != nil {
		return nil, "", err
	}
	if _, err = io.Copy(fileWriter, file); err != nil {
		return nil, "", err
	}
	return &bodyBuf, bodyWriter.FormDataContentType(), nil
}
