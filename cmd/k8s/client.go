package k8s

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/spf13/viper"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func getConfig() (*rest.Config, string, error) {
	kubeconfig := viper.GetString("kubeconfig")
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".shipyard", "kubeconfig")
			log.Println("Using a kubeconfig found in the default shipyard location.")
		} else {
			return nil, "", fmt.Errorf("no kubeconfig file path provided")
		}
	}

	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
		nil)

	rawConfig, err := cfg.RawConfig()
	if err != nil {
		return nil, "", err
	}

	contexts := rawConfig.Contexts
	if len(contexts) == 0 {
		return nil, "", fmt.Errorf("kubeconfig does not have a context set")
	}
	namespace := contexts[rawConfig.CurrentContext].Namespace

	restClientConfig, err := cfg.ClientConfig()
	if err != nil {
		return nil, "", err
	}

	return restClientConfig, namespace, nil
}
