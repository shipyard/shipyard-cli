package env

import (
	"fmt"

	"github.com/pkg/browser"
	"github.com/shipyard/shipyard-cli/constants"
	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/spf13/cobra"
)

func NewVisitCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "visit",
		GroupID:      constants.GroupEnvironments,
		SilenceUsage: true,
		Short:        "Visit an environment's latest build",
		Long:         `This command opens a web browser to let users visit a given environment`,
		Example: `  # Visit the current build for environment ID 12345
  shipyard visit 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return visitEnvironment(c, args[0])
			}
			return errNoEnvironment
		},
	}

	return cmd
}

func visitEnvironment(c client.Client, id string) error {
	e, err := c.EnvByID(id)
	if err != nil {
		return err
	}

	url := e.Data.Attributes.URL
	if url == "" {
		return fmt.Errorf("no URL found for environment %s", id)
	}

	if err := browser.OpenURL(url); err != nil {
		return fmt.Errorf("unable to open a web browser, visit the environment at %s", url)
	}
	return nil
}
