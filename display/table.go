package display

import (
	"io"

	"github.com/olekukonko/tablewriter"
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
