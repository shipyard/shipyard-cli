package telepresence

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/k8s"
)

func NewTelepresenceCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use: "telepresence",
	}
	cmd.AddCommand(NewConnectCmd(c))
	return cmd
}

func NewConnectCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connect",
		Aliases: []string{"c"},
		Short:   "Connect to an environment via telepresence",
		Example: `  # Connect telepresence to env ID 12345:
  shipyard telepresence connect 12345`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			_ = viper.BindPFlag("env", cmd.Flags().Lookup("env"))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return connect(c)
		},
	}
	cmd.Flags().String("env", "", "environment ID")
	_ = cmd.MarkFlagRequired("env")
	return cmd
}

func connect(c client.Client) error {
	if _, err := exec.LookPath("telepresence"); err != nil {
		return fmt.Errorf("telepresence not found, please make sure it's in your PATH")
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
	).CombinedOutput()

	if err != nil {
		display.Println(string(out))
		return err
	}
	return nil
}
