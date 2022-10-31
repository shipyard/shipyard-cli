package k8s

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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

func getPodName(config *rest.Config, namespace string, deployment string) (string, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", err
	}

	options := metav1.ListOptions{
		LabelSelector: "component=" + deployment,
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), options)
	if err != nil {
		return "", err
	}

	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pod found for service %s", deployment)
	}

	return pods.Items[0].Name, nil
}
