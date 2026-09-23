package k8s

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

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
	utilexec "k8s.io/client-go/util/exec"

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

	// No deadline: an interactive session lasts as long as the user wants it.
	return exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin:             in,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		TerminalSizeQueue: &fixedTerminalSizeQueue{},
	})
}

// ExecOutput is what a non-interactive command wrote before it finished.
type ExecOutput struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Truncated bool
}

// ExecCapture runs a command in the service's pod and returns what it wrote.
//
// This is Exec without the terminal: no stdin is attached and no TTY is
// requested, so it fits a caller that has one request and wants one response,
// such as the MCP server. Dropping the TTY also keeps stdout and stderr on
// separate streams, which a TTY merges.
//
// A command that exits non-zero is not an error here: its output and exit code
// are the answer. An error means the exec never ran or the stream broke.
//
// maxBytes caps each stream; zero or less means no cap. Cap what you hand to a
// model: `cat` on a large file otherwise fills its context with one tool result.
func (c *Service) ExecCapture(ctx context.Context, args []string, maxBytes int) (ExecOutput, error) {
	req := c.clientSet.CoreV1().RESTClient().Post().Resource("pods").Name(c.pod).
		Namespace(c.namespace).SubResource("exec")
	option := &v1.PodExecOptions{
		Command: args,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     false,
	}

	req.VersionedParams(option, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(c.restConfig, "POST", req.URL())
	if err != nil {
		return ExecOutput{}, err
	}

	stdout := &cappedBuffer{limit: maxBytes}
	stderr := &cappedBuffer{limit: maxBytes}

	// StreamWithContext closes the connection when ctx ends, so a timeout frees
	// the stream instead of leaving it copying in the background. It does not
	// kill the process in the pod: with no TTY to hang up, a process that writes
	// again dies on the closed pipe, and one that never writes runs until it
	// exits. The copy can still be mid-write as this returns, which is why the
	// buffers are mutex-guarded.
	streamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
	})
	// A command that finished as the deadline passed still has a real result.
	if ctxErr := ctx.Err(); streamErr != nil && ctxErr != nil {
		return ExecOutput{
			Stdout:    stdout.String(),
			Stderr:    stderr.String(),
			Truncated: stdout.Truncated() || stderr.Truncated(),
		}, ctxErr
	}

	out := ExecOutput{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Truncated: stdout.Truncated() || stderr.Truncated(),
	}

	if streamErr != nil {
		// The command ran and exited non-zero: report that as a result, the way
		// a shell does, rather than losing the output to an error return.
		var exitErr utilexec.CodeExitError
		if errors.As(streamErr, &exitErr) {
			out.ExitCode = exitErr.Code
			return out, nil
		}

		return out, streamErr
	}

	return out, nil
}

// cappedBuffer collects up to limit bytes and counts the rest as truncated. It
// is safe for concurrent use: the stream copy writes to it from its own
// goroutine while a timed-out ExecCapture reads what arrived.
type cappedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (w *cappedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.buf.String()
}

func (w *cappedBuffer) Truncated() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.truncated
}

func (w *cappedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.limit <= 0 {
		return w.buf.Write(p)
	}

	room := w.limit - w.buf.Len()
	if w.truncated || room <= 0 {
		// Report the whole write as accepted: the caller is a stream copy that
		// treats a short write as an error and would abort the exec.
		w.truncated = true
		return len(p), nil
	}

	if len(p) > room {
		w.truncated = true

		// Cut at a character boundary: half a UTF-8 sequence becomes U+FFFD
		// once the output is marshalled to JSON.
		cut := room
		for cut > 0 && !utf8.RuneStart(p[cut]) {
			cut--
		}
		if _, err := w.buf.Write(p[:cut]); err != nil {
			return 0, err
		}
		return len(p), nil
	}

	return w.buf.Write(p)
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

// GetLogsAsString returns logs as a string instead of printing them
// This is used by the MCP logs service to capture log output
func (c *Service) GetLogsAsString(follow bool, tail int64) (string, error) {
	opts := v1.PodLogOptions{
		Follow:    follow,
		TailLines: &tail,
	}
	req := c.clientSet.CoreV1().Pods(c.namespace).GetLogs(c.pod, &opts)

	podLogs, err := req.Stream(context.TODO())
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	var buf bytes.Buffer
	if _, err = io.Copy(&buf, podLogs); err != nil {
		return "", err
	}

	return buf.String(), nil
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
