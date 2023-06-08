package telepresence

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/k8s"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var errNoEnvironment = errors.New("environment ID argument not provided")

func NewConnectCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "telepresence connect [environment ID]",
		Aliases: []string{"env"},
		Short:   "Connect telepresence to an environment by ID",
		Example: `  # Connect telepresence to env ID 12345:
  shipyard telepresence connect 12345
  
  # Get all the details for environment ID 12345 in JSON format:
  shipyard get environment 12345 --json`,
		SilenceUsage: true,
		// Due to an issue in viper, bind the 'json' flag in PreRun for each command that uses
		// a flag name already bound to a sibling command.
		// See https://github.com/spf13/viper/issues/233#issuecomment-386791444
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return connect(c)
			}
			return errNoEnvironment
		},
	}

	cmd.Flags().String("env", "", "Environment ID")
	_ = cmd.MarkFlagRequired("env")

	return cmd
}

func connect(c client.Client) error {
	if _, err := exec.LookPath("telepresence"); err != nil {
		return fmt.Errorf("telepresence not found, please make sure telepresence is in your path")
	}

	id := viper.GetString("env")

	k, err := k8s.NewConfig(c, id)
	if err != nil {
		return err
	}

	out, err := exec.Command( // nolint:gosec // this comes from k8s.NewConfig, so its fine.
		"telepresence",
		"connect",
		fmt.Sprintf(
			"--kubeconfig=%s",
			k.Path,
		),
	).Output()

	if err != nil {
		fmt.Println(out)
		fmt.Println(err)
		return err
	}

	return nil
}
