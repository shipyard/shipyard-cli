package k8s

import (
	"fmt"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	restConfig *rest.Config
	clientSet  *kubernetes.Clientset
	Path       string
}

func NewConfig(c client.Client, envid string) (*Client, error) {
	if err := setupKubeconfig(c, envid); err != nil {
		return nil, err
	}

	path, err := kubeconfigPath()
	if err != nil {
		return nil, err
	}
	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{
			ExplicitPath: path,
		},
		nil,
	)

	rawConfig, err := cfg.RawConfig()
	if err != nil {
		return nil, err
	}

	contexts := rawConfig.Contexts
	if len(contexts) == 0 {
		return nil, fmt.Errorf("kubeconfig does not have a context set")
	}
	restConfig, err := cfg.ClientConfig()
	if err != nil {
		return nil, err
	}

	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	sc := Client{
		restConfig: restConfig,
		clientSet:  clientSet,
		Path:       path,
	}

	return &sc, nil
}
