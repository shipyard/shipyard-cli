package stop

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop an environment",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	}

	cmd.AddCommand(newEnvironmentCmd())

	return cmd
}

func newEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"env"},
		Use:     "environment",
		Short:   "Stop a running environment",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("stop env called")
		},
	}

	return cmd
}
