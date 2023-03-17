package k8s

import (
	"errors"
	"os"

	"github.com/docker/cli/cli/streams"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/shipyard/shipyard-cli/constants"
)

func NewExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "exec",
		GroupID: constants.GroupEnvironments,
		Short:   "Execute a command in a service in an environment",
		Long: `Execute any command with any arguments and flags in a given service.
You can also run interactive commands, like shells, without passing anything special to exec.

Pass any command arguments after a double slash.

shipyard exec --env 123 --service web -- ls -l -a
shipyard exec --env 123 --service web -- bash`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("service", cmd.Flags().Lookup("service"))
			viper.BindPFlag("env", cmd.Flags().Lookup("env"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleExecCmd(args)
		},
	}

	cmd.Flags().String("service", "", "Service name")
	cmd.MarkFlagRequired("service")

	cmd.Flags().String("env", "", "Environment ID")
	cmd.MarkFlagRequired("env")

	return cmd
}

func handleExecCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("no command arguments provided")
	}

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

	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
		Namespace(namespace).SubResource("exec")

	option := &v1.PodExecOptions{
		Command: args,
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

// fixedTerminalSizeQueue and its Next method ensure the terminal size remains the same
// after being attached to and detached from a shell in a container.
type fixedTerminalSizeQueue struct{}

func (s *fixedTerminalSizeQueue) Next() *remotecommand.TerminalSize {
	return &remotecommand.TerminalSize{
		Width:  3000,
		Height: 8000,
	}
}
