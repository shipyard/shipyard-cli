package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/docker/cli/cli/streams"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/types"
)

type Service struct {
	restConfig *rest.Config
	clientSet  *kubernetes.Clientset
	client     client.Client
	namespace  string
	pod        string
}

func New(c client.Client, id string, svc *types.Service) (*Service, error) {
	s := Service{client: c}
	if err := setupKubeconfig(c, id); err != nil {
		return nil, err
	}

	path, err := kubeconfigPath()
	if err != nil {
		return nil, err
	}

	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: path},
		nil)

	rawConfig, err := cfg.RawConfig()
	if err != nil {
		return nil, err
	}

	contexts := rawConfig.Contexts
	if len(contexts) == 0 {
		return nil, fmt.Errorf("kubeconfig does not have a context set")
	}
	s.namespace = contexts[rawConfig.CurrentContext].Namespace

	restConfig, err := cfg.ClientConfig()
	if err != nil {
		return nil, err
	}
	s.restConfig = restConfig

	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	s.clientSet = clientSet

	pod, err := s.podForService(svc)
	if err != nil {
		return nil, err
	}
	s.pod = pod

	return &s, nil
}

func (c *Service) Exec(args []string) error {
	req := c.clientSet.CoreV1().RESTClient().Post().Resource("pods").Name(c.pod).
		Namespace(c.namespace).SubResource("exec")
	option := &v1.PodExecOptions{
		Command: args,
		Stdin:   true,
		Stdout:  true,
		Stderr:  true,
		TTY:     true,
	}

	req.VersionedParams(option, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(c.restConfig, "POST", req.URL())
	if err != nil {
		return err
	}

	in := streams.NewIn(os.Stdin)
	if err := in.SetRawTerminal(); err != nil {
		return err
	}
	defer in.RestoreTerminal()

	return exec.Stream(remotecommand.StreamOptions{
		Stdin:             in,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		TerminalSizeQueue: &fixedTerminalSizeQueue{},
	})
}

func (c *Service) Logs(follow bool, tail int64) error {
	opts := v1.PodLogOptions{
		Follow:    follow,
		TailLines: &tail,
	}
	req := c.clientSet.CoreV1().Pods(c.namespace).GetLogs(c.pod, &opts)

	podLogs, err := req.Stream(context.TODO())
	if err != nil {
		return err
	}
	defer func() { _ = podLogs.Close() }()

	if !follow {
		var buf bytes.Buffer
		if _, err = io.Copy(&buf, podLogs); err != nil {
			return err
		}
		display.Print(buf.String())
		return nil
	}

	for {
		buf := make([]byte, 2000)
		bytesRead, err := podLogs.Read(buf)
		if bytesRead == 0 {
			continue
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		message := string(buf[:bytesRead])
		display.Print(message)
	}

	return nil
}

func (c *Service) PortForward(ports []string) error {
	roundTripper, upgrader, err := spdy.RoundTripperFor(c.restConfig)
	if err != nil {
		return err
	}

	host := strings.TrimPrefix(c.restConfig.Host, "https://")
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", c.namespace, c.pod)
	serverURL := url.URL{Scheme: "https", Host: host, Path: path}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, &serverURL)
	stopChan, readyChan := make(chan struct{}, 1), make(chan struct{}, 1)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)

	forwarder, err := portforward.New(dialer, ports, stopChan, readyChan, out, errOut)
	if err != nil {
		return err
	}

	go func() {
		for range readyChan {
		}

		if s := errOut.String(); s != "" {
			display.Fail(s)
		} else if s = out.String(); s != "" {
			display.Print(s)
		}
	}()

	if err := forwarder.ForwardPorts(); err != nil {
		return err
	}
	return nil
}

// podForService uses the service's sanitized name to find the pod in a given namespace.
func (c *Service) podForService(svc *types.Service) (string, error) {
	options := metav1.ListOptions{
		LabelSelector: "component=" + svc.SanitizedName,
	}

	pods, err := c.clientSet.CoreV1().Pods(c.namespace).List(context.TODO(), options)
	if err != nil {
		return "", err
	}

	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pod found for service %s", svc.Name)
	}
	return pods.Items[0].Name, nil
}

// fixedTerminalSizeQueue and its Next method ensure the terminal size remains the same
// after being attached to and detached from a shell in a container.
type fixedTerminalSizeQueue struct{}

func (s *fixedTerminalSizeQueue) Next() *remotecommand.TerminalSize {
	return &remotecommand.TerminalSize{
		Width:  3000,
		Height: 8000,
	}
}
