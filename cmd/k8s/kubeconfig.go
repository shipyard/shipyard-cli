package k8s

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"k8s.io/client-go/util/homedir"

	"shipyard/requests"
	"shipyard/requests/uri"
)

// SetKubeconfig tries to fetch a kubeconfig for a given environment and
// save it in the default store directory.
func SetKubeconfig(envID string) error {
	cfg, err := getKubeconfig(envID)
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}
	if err = saveKubeconfig(cfg); err != nil {
		return fmt.Errorf("failed to save kubeconfig: %w", err)
	}
	return nil
}

// getKubeconfig tries to fetch the kubeconfig from the backend API.
func getKubeconfig(envID string) ([]byte, error) {
	client, err := requests.NewHTTPClient(os.Stdout)
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	org := viper.GetString("org")
	if org != "" {
		params["org"] = org
	}

	uri := uri.CreateResourceURI("", "environment", envID, "kubeconfig", params)
	body, err := client.Do(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func saveKubeconfig(body []byte) error {
	var p string
	var err error

	home := homedir.HomeDir()
	if home != "" {
		p = filepath.Join(home, ".shipyard", "kubeconfig")
		if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return fmt.Errorf("failed to create the .shipyard directory in $HOME: %v", err)
		}
	}

	return os.WriteFile(p, body, 0644)
}
