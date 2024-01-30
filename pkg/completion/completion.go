package completion

import (
	"github.com/shipyard/shipyard-cli/pkg/client"

	"github.com/spf13/cobra"
)

type Completion struct {
	client client.Client
}

func New(cl client.Client) Completion {
	return Completion{client: cl}
}

func (c Completion) EnvironmentUUIDs(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	resp, err := c.client.AllEnvironmentUUIDs()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	ids := make([]string, len(resp.Data))
	for i := range resp.Data {
		ids[i] = resp.Data[i]
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}
