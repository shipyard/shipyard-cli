package k8s

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"k8s.io/client-go/util/homedir"

	"github.com/shipyard/shipyard-cli/pkg/requests"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

// SetupKubeconfig tries to fetch a kubeconfig for a given environment and
// save it in the default store directory.
func SetupKubeconfig(envID, org string) error {
	cfg, err := fetchKubeconfig(envID, org)
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}
	if err = saveKubeconfig(cfg); err != nil {
		return fmt.Errorf("failed to save kubeconfig: %w", err)
	}
	return nil
}

// fetchKubeconfig tries to fetch the Kubeconfig from the backend API.
func fetchKubeconfig(envID, org string) ([]byte, error) {
	client, err := requests.New(io.Discard)
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	if org != "" {
		params["org"] = org
	}

	requestURI := uri.CreateResourceURI("", "environment", envID, "kubeconfig", params)
	body, err := client.Do(http.MethodGet, requestURI, nil)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// saveKubeconfig persists a slice of bytes that contains the Kubeconfig file
// to disk in the HOME directory of the user.
func saveKubeconfig(body []byte) error {
	var p string
	var err error

	home := homedir.HomeDir()
	if home != "" {
		p = filepath.Join(home, ".shipyard", "kubeconfig")
		if err = os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("failed to create the .shipyard directory in $HOME: %v", err)
		}
	}
	return os.WriteFile(p, body, 0o600)
}

// kubeconfigPath tries to find a Kubeconfig in the HOME directory of the user.
func kubeconfigPath() (string, error) {
	if home := homedir.HomeDir(); home != "" {
		kubeconfigPath := filepath.Join(home, ".shipyard", "kubeconfig")
		if _, err := os.Stat(kubeconfigPath); err != nil {
			return "", err
		}
		log.Println("Using a kubeconfig found in the default .shipyard directory")
		return kubeconfigPath, nil
	}
	return "", fmt.Errorf("user's $HOME directory not found")
}
