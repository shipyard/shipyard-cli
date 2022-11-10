package k8s

import (
	"os"

	"github.com/docker/cli/cli/streams"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

func NewExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec",
		Short: "Execute a command in a service in an environment",
		Long: `Execute any command with any arguments and flags in a given service.
You can also run interactive commands, like shells, without passing anything special to exec.`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("service", cmd.Flags().Lookup("service"))
			viper.BindPFlag("env", cmd.Flags().Lookup("env"))
			viper.BindPFlag("cmd", cmd.Flags().Lookup("cmd"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleExecCmd()
		},
	}

	cmd.Flags().String("service", "", "Service name")
	cmd.MarkFlagRequired("service")

	cmd.Flags().String("env", "", "Environment ID")
	cmd.MarkFlagRequired("env")

	cmd.Flags().StringSlice("cmd", nil, "Command (comma-separated, like 'ls,-l,-a')")
	cmd.MarkFlagRequired("cmd")

	return cmd
}

func handleExecCmd() error {
	if err := SetKubeconfig(viper.GetString("env")); err != nil {
		return err
	}

	config, namespace, err := getConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	serviceName := viper.GetString("service")
	podName, err := getPodName(config, namespace, serviceName)
	if err != nil {
		return err
	}

	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
		Namespace(namespace).SubResource("exec")

	option := &v1.PodExecOptions{
		Command: viper.GetStringSlice("cmd"),
		Stdin:   true,
		Stdout:  true,
		Stderr:  true,
		TTY:     true,
	}

	req.VersionedParams(option, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
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

type fixedTerminalSizeQueue struct{}

func (s *fixedTerminalSizeQueue) Next() *remotecommand.TerminalSize {
	return &remotecommand.TerminalSize{
		Width:  3000,
		Height: 8000,
	}
}
