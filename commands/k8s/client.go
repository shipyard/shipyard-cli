package k8s

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/shipyard/shipyard-cli/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// getKubeconfigPath tries to find a kubeconfig in the HOME directory of the user.
func getKubeconfigPath() (string, error) {
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

// getRESTConfig tries to find a kubeconfig, extract a namespace in the current context,
// and create a rest.Config from the kubeconfig.
func getRESTConfig() (*rest.Config, string, error) {
	kubeconfigPath, err := getKubeconfigPath()
	if err != nil {
		return nil, "", err
	}

	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
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

// getPodName uses the service's sanitized name to find the pod in a given namespace.
func getPodName(clientSet *kubernetes.Clientset, namespace string, svc *types.Service) (string, error) {
	options := metav1.ListOptions{
		LabelSelector: "component=" + svc.SanitizedName,
	}

	pods, err := clientSet.CoreV1().Pods(namespace).List(context.TODO(), options)
	if err != nil {
		return "", err
	}

	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pod found for service %s", svc.Name)
	}

	return pods.Items[0].Name, nil
}
