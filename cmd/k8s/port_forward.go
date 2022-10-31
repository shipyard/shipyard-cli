package k8s

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"shipyard/constants"
	"shipyard/display"
)

func NewPortForwardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "port-forward",
		Aliases: []string{"pf"},
		Short:   "Port-forward to a service in an environment",
		GroupID: constants.GroupKubernetes,
		PreRun: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("kubeconfig", cmd.Flags().Lookup("kubeconfig"))
			viper.BindPFlag("ports", cmd.Flags().Lookup("ports"))
			viper.BindPFlag("service", cmd.Flags().Lookup("service"))
			viper.BindPFlag("env", cmd.Flags().Lookup("env"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handlePortForwardCmd()
		},
	}

	cmd.Flags().String("kubeconfig", "", "Path to kubeconfig")

	cmd.Flags().StringSlice("ports", nil, "Ports (for example, 3000:80)")
	cmd.MarkFlagRequired("ports")

	cmd.Flags().String("service", "", "Service name")
	cmd.MarkFlagRequired("service")

	cmd.Flags().String("env", "", "environment ID")
	cmd.MarkFlagRequired("env")

	return cmd
}

func handlePortForwardCmd() error {
	if err := SetKubeconfig(viper.GetString("env")); err != nil {
		return err
	}

	config, namespace, err := getConfig()
	if err != nil {
		return err
	}

	ports := viper.GetStringSlice("ports")
	serviceName := viper.GetString("service")

	return portForward(config, ports, namespace, serviceName)
}

func portForward(config *rest.Config, ports []string, namespace string, serviceName string) error {
	podName, err := getPodName(config, namespace, serviceName)
	if err != nil {
		return err
	}

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
