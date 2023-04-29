package k8s

import (
	"bytes"
	"context"
	"io"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/k8s"
)

func NewLogsCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs",
		GroupID: constants.GroupEnvironments,
		Aliases: []string{"log"},
		Short:   "Get logs from a service in an environment",
		Example: `  # Get logs for service flask-backend:
  shipyard logs --env 12345 --service flask-backend
  
  # Follow logs for the flask-backend service:
  shipyard logs --env 12345 --service flask-backend --follow

  # Get last 100 lines of logs for the flask-backend service:
  shipyard logs --env 12345 --service flask-backend --tail 100`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("service", cmd.Flags().Lookup("service"))
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
			_ = viper.BindPFlag("follow", cmd.Flags().Lookup("follow"))
			_ = viper.BindPFlag("tail", cmd.Flags().Lookup("tail"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleLogsCmd(c)
		},
	}

	cmd.Flags().String("service", "", "Service name")
	_ = cmd.MarkFlagRequired("service")

	cmd.Flags().String("env", "", "Environment ID")
	_ = cmd.MarkFlagRequired("env")

	cmd.Flags().BoolP("follow", "f", false, "Follow the log output")
	cmd.Flags().Int64("tail", 3000, "Number of lines from the end of the logs to show")

	return cmd
}

func handleLogsCmd(c client.Client) error {
	serviceName := viper.GetString("service")
	id := viper.GetString("env")

	s, err := c.FindService(serviceName, id)
	if err != nil {
		return err
	}

	if err := k8s.SetupKubeconfig(id, c.Org); err != nil {
		return err
	}

	config, namespace, err := k8s.RESTConfig()
	if err != nil {
		return err
	}

	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	podName, err := k8s.PodName(clientSet, namespace, s)
	if err != nil {
		return err
	}

	follow := viper.GetBool("follow")
	tail := viper.GetInt64("tail")

	podLogOpts := corev1.PodLogOptions{
		Follow:    follow,
		TailLines: &tail,
	}
	req := clientSet.CoreV1().Pods(namespace).GetLogs(podName, &podLogOpts)
	podLogs, err := req.Stream(context.TODO())
	if err != nil {
		return err
	}
	defer podLogs.Close()

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
