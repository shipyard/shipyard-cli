package k8s

import (
	"bytes"
	"context"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"shipyard/display"
)

func NewLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "logs",
		Short:        "Get logs from a service in an environment",
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag("service", cmd.Flags().Lookup("service"))
			viper.BindPFlag("env", cmd.Flags().Lookup("env"))
			viper.BindPFlag("follow", cmd.Flags().Lookup("follow"))
			viper.BindPFlag("lines", cmd.Flags().Lookup("lines"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleLogsCmd()
		},
	}

	cmd.Flags().String("service", "", "Service name")
	cmd.MarkFlagRequired("service")

	cmd.Flags().String("env", "", "Environment ID")
	cmd.MarkFlagRequired("env")

	cmd.Flags().Bool("follow", false, "Follow the log output")
	cmd.Flags().Int64("lines", 3000, "Number of lines from the end of the logs to show")

	return cmd
}

func handleLogsCmd() error {
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
	podName, err := getPodName(config, namespace, serviceName)
	if err != nil {
		return err
	}

	follow := viper.GetBool("follow")
	lines := viper.GetInt64("lines")

	podLogOpts := corev1.PodLogOptions{
		Follow:    follow,
		TailLines: &lines,
	}
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &podLogOpts)
	podLogs, err := req.Stream(context.TODO())
	if err != nil {
		return err
	}
	defer podLogs.Close()

	writer := display.NewSimpleDisplay()

	if !follow {
		var buf bytes.Buffer
		_, err = io.Copy(&buf, podLogs)
		if err != nil {
			return err
		}
		writer.Output(buf.String())
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
		writer.Output(message)
	}

	return nil
}
