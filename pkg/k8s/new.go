package k8s

import (
	"fmt"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"k8s.io/client-go/tools/clientcmd"
)

// Client points at the environment's kubeconfig saved in ~/.shipyard, for
// tools such as telepresence that need a file rather than a parsed config.
type Client struct {
	Path string
}

// NewConfig saves the environment's kubeconfig to the shared
// ~/.shipyard/kubeconfig and returns its path. Only telepresence uses it;
// logs, exec and port-forward keep the config in memory (see New).
func NewConfig(c client.Client, envid string) (*Client, error) {
	if err := setupKubeconfig(c, envid); err != nil {
		return nil, err
	}

	path, err := kubeconfigPath()
	if err != nil {
		return nil, err
	}

	rawConfig, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return nil, err
	}
	if len(rawConfig.Contexts) == 0 {
		return nil, fmt.Errorf("kubeconfig does not have a context set")
	}

	return &Client{Path: path}, nil
}
