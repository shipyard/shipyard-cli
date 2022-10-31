package k8s

import (
	"github.com/spf13/cobra"

	"shipyard/constants"
)

func NewExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "exec",
		Short:   "Execute a command in a pod in an environment",
		GroupID: constants.GroupKubernetes,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	return cmd
}
