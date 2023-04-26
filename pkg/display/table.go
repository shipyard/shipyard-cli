package display

import (
	"fmt"
	"io"
	"strconv"

	"github.com/olekukonko/tablewriter"

	"github.com/shipyard/shipyard-cli/pkg/types"
)

// RenderTable writes data in tabular form with given column names to the provided writer.
func RenderTable(out io.Writer, columns []string, data [][]string) {
	table := tablewriter.NewWriter(out)
	table.SetHeader(columns)

	table.SetAutoMergeCellsByColumnIndex([]int{0, 1, 5})
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetBorder(false)
	table.SetHeaderLine(true)
	table.SetTablePadding("\t")

	for _, v := range data {
		table.Append(v)
	}
	table.Render()
	_, _ = io.WriteString(out, "\n")
}

// FormattedEnvironment takes an environment, extracts data from it, and prepares it
// to be in tabular format. If the environment value is nil, the program will panic.
func FormattedEnvironment(env *types.Environment) [][]string {
	data := make([][]string, 0, len(env.Attributes.Projects))

	for _, p := range env.Attributes.Projects {
		pr := strconv.Itoa(p.PullRequestNumber)
		if pr == "0" {
			pr = ""
		}

		data = append(data, []string{
			env.Attributes.Name,
			env.ID,
			fmt.Sprintf("%t", env.Attributes.Ready),
			p.RepoName,
			pr,
			env.Attributes.URL,
		})
	}

	return data
}
