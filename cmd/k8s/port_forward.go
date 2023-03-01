package k8s

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/display"
)

func NewPortForwardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "port-forward",
		GroupID: constants.GroupEnvironments,
		Aliases: []string{"pf"},
		Short:   "Port-forward to a service in an environment",
		Example: `  # Get an environment's services and exposed ports:
  shipyard get services --env 12345

  # port-forward "web" service's port 80:
  shipyard port-forward --env 12345 --service web --ports 80:80`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("ports", cmd.Flags().Lookup("ports"))
			viper.BindPFlag("service", cmd.Flags().Lookup("service"))
			viper.BindPFlag("env", cmd.Flags().Lookup("env"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handlePortForwardCmd()
		},
	}

	cmd.Flags().StringSlice("ports", nil, "Ports (for example, 3000:80)")
	cmd.MarkFlagRequired("ports")

	cmd.Flags().String("service", "", "Service name")
	cmd.MarkFlagRequired("service")

	cmd.Flags().String("env", "", "Environment ID")
	cmd.MarkFlagRequired("env")

	return cmd
}

func handlePortForwardCmd() error {
	if err := SetKubeconfig(viper.GetString("env")); err != nil {
		return err
	}

	config, namespace, err := getRESTConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	serviceName := viper.GetString("service")
	podName, err := getPodName(clientset, namespace, serviceName)
	if err != nil {
		return err
	}

	ports := viper.GetStringSlice("ports")
	return portForward(config, namespace, podName, ports)
}

func portForward(config *rest.Config, namespace, pod string, ports []string) error {
	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return err
	}

	host := strings.TrimPrefix(config.Host, "https://")
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, pod)
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

		if s := errOut.String(); s != "" {
			writer.Fail(s)
		} else if s = out.String(); s != "" {
			writer.Print(s)
		}
	}()

	if err := forwarder.ForwardPorts(); err != nil {
		return err
	}
	return nil
}
