package k8s

import (
	"bytes"
	"fmt"
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
	"shipyard/logging"
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
	viper.BindPFlag("ports", cmd.Flags().Lookup("ports"))

	cmd.Flags().String("pod", "", "Pod name")
	viper.BindPFlag("pod", cmd.Flags().Lookup("pod"))

	return cmd
}

func handlePortForwardCmd() error {
	kubeconfig := viper.GetString("kubeconfig")
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
			logging.LogIfVerbose("Using a Kubeconfig found in the default location.")
		} else {
			return fmt.Errorf("no Kubeconfig file path provided")
		}
	}

	ports := viper.GetStringSlice("ports")
	if len(ports) == 0 {
		return fmt.Errorf("no ports provided")
	}

	podName := viper.GetString("pod")
	if podName == "" {
		return fmt.Errorf("no pod name provided")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}

	return portForward(config, ports, podName)
}

// TODO: figure out what exact namespace to use.
// shipyard-app-build-{UUID of build}
func portForward(config *rest.Config, ports []string, podName string) error {
	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	host := strings.TrimLeft(config.Host, "https://")
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", "default", podName)
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
