package k8s

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/homedir"

	"shipyard/display"
)

func NewPortForwardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "port-forward",
		Aliases: []string{"pf"},
		Short:   "Port-forward to a pod in an environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return handlePortForwardCmd()
		},
	}

	cmd.Flags().String("kubeconfig", "", "Path to Kubeconfig")
	viper.BindPFlag("kubeconfig", cmd.Flags().Lookup("kubeconfig"))

	cmd.Flags().StringSlice("ports", nil, "Ports (for example, 3000:80)")
	cmd.MarkFlagRequired("ports")
	viper.BindPFlag("ports", cmd.Flags().Lookup("ports"))

	cmd.Flags().String("pod", "", "Pod name")
	cmd.MarkFlagRequired("pod")
	viper.BindPFlag("pod", cmd.Flags().Lookup("pod"))

	cmd.Flags().String("env", "", "env ID")
	cmd.MarkFlagRequired("env")
	viper.BindPFlag("env", cmd.Flags().Lookup("env"))

	return cmd
}

func handlePortForwardCmd() error {
	if err := SetKubeconfig(viper.GetString("env")); err != nil {
		return err
	}

	kubeconfig := viper.GetString("kubeconfig")
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".shipyard", "kubeconfig")
			log.Println("Using a kubeconfig found in the default shipyard location.")
		} else {
			return fmt.Errorf("no kubeconfig file path provided")
		}
	}

	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
		nil)

	rawConfig, err := cfg.RawConfig()
	if err != nil {
		return err
	}

	contexts := rawConfig.Contexts
	if len(contexts) == 0 {
		return fmt.Errorf("kubeconfig does not have a context set")
	}
	namespace := contexts[rawConfig.CurrentContext].Namespace

	restClientConfig, err := cfg.ClientConfig()
	if err != nil {
		return err
	}

	ports := viper.GetStringSlice("ports")
	podName := viper.GetString("pod")

	return portForward(restClientConfig, ports, namespace, podName)
}

func portForward(config *rest.Config, ports []string, namespace string, podName string) error {
	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	host := strings.TrimLeft(config.Host, "https://")
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)
	serverURL := url.URL{Scheme: "https", Host: host, Path: path}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, &serverURL)

	stopChan, readyChan := make(chan struct{}, 1), make(chan struct{}, 1)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)

	forwarder, err := portforward.New(dialer, ports, stopChan, readyChan, out, errOut)
	if err != nil {
		return err
	}

	writer := display.NewSimpleDisplay()

	go func() {
		for range readyChan {
		}

		if s := errOut.String(); len(s) != 0 {
			writer.Fail(s)
		} else if s = out.String(); len(s) != 0 {
			writer.Output(s)
		}
	}()

	if err = forwarder.ForwardPorts(); err != nil {
		return err
	}
	return nil
}
