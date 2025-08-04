package display

import (
	"hash/fnv"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/mattn/go-isatty"

	"github.com/shipyard/shipyard-cli/pkg/types"
)

// RenderTable writes data in tabular form with given column names to the provided writer.
func RenderTable(out io.Writer, columns []string, data [][]string) {
	t := table.NewWriter()
	t.SetOutputMirror(out)

	// Set header
	headerRow := table.Row{}
	for _, col := range columns {
		headerRow = append(headerRow, col)
	}
	t.AppendHeader(headerRow)

	// Add data rows
	for _, row := range data {
		dataRow := table.Row{}
		for i, cell := range row {
			// Ensure we don't exceed the number of columns
			if i < len(columns) {
				dataRow = append(dataRow, cell)
			}
		}
		// Ensure we have the right number of columns - pad with empty strings if needed
		for len(dataRow) < len(columns) {
			dataRow = append(dataRow, "")
		}
		t.AppendRow(dataRow)
	}

	// Configure table style
	t.SetStyle(table.Style{
		Name: "CustomStyle",
		Box: table.BoxStyle{
			BottomLeft:       "",
			BottomRight:      "",
			BottomSeparator:  "",
			Left:             "",
			LeftSeparator:    "",
			MiddleHorizontal: "-",
			MiddleSeparator:  "",
			MiddleVertical:   "",
			PaddingLeft:      "",
			PaddingRight:     "\t",
			Right:            "",
			RightSeparator:   "",
			TopLeft:          "",
			TopRight:         "",
			TopSeparator:     "",
			UnfinishedRow:    "",
		},
		Color:  table.ColorOptions{},
		Format: table.FormatOptions{},
		HTML:   table.HTMLOptions{},
		Options: table.Options{
			DrawBorder:      false,
			SeparateColumns: false,
			SeparateFooter:  false,
			SeparateHeader:  true,
			SeparateRows:    false,
		},
		Title: table.TitleOptions{},
	})

	t.Render()
	_, _ = io.WriteString(out, "\n")
}

// FormatReadyStatus formats a boolean Ready status with colors
func FormatReadyStatus(ready bool) string {
	if ready {
		green := color.New(color.FgGreen)
		return green.Sprint("Yes")
	}
	red := color.New(color.FgRed)
	return red.Sprint("No")
}

// supportsOSC8 detects if the current terminal supports OSC 8 hyperlinks
func supportsOSC8() bool {
	// Check if we're in a terminal
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return false
	}

	termProgram := os.Getenv("TERM_PROGRAM")
	term := os.Getenv("TERM")

	// Known terminals that support OSC 8
	supportedTerminals := map[string]bool{
		"iTerm.app":        true,
		"WezTerm":          true,
		"Alacritty":        true,
		"kitty":            true,
		"Hyper":            true,
		"tabby":            true,
		"Terminus":         true,
		"vscode":           true,
		"Windows Terminal": true,
	}

	if supportedTerminals[termProgram] {
		return true
	}

	// Check for specific terminal features
	if strings.Contains(term, "kitty") ||
		strings.Contains(term, "xterm-kitty") ||
		termProgram == "gnome-terminal" ||
		termProgram == "konsole" ||
		os.Getenv("KONSOLE_VERSION") != "" ||
		os.Getenv("VTE_VERSION") != "" {
		return true
	}

	// Apple Terminal and most basic terminals don't support OSC 8
	if termProgram == "Apple_Terminal" || term == "xterm-256color" {
		return false
	}

	// Default to false for unknown terminals
	return false
}

// FormatClickableURL formats a URL as a clickable terminal link using OSC 8 escape sequences
// Falls back to underlined turquoise URL if terminal doesn't support OSC 8
func FormatClickableURL(url string) string {
	if url == "" {
		return ""
	}

	if supportsOSC8() {
		// OSC 8 escape sequence: \033]8;;URL\033\\TEXT\033]8;;\033\\
		return "\033]8;;" + url + "\033\\" + url + "\033]8;;\033\\"
	}

	// Fallback: return underlined turquoise URL
	cyan := color.New(color.FgCyan, color.Underline)
	return cyan.Sprint(url)
}

// FormatColoredAppName assigns a consistent color to app names based on hash
func FormatColoredAppName(appName string) string {
	if appName == "" {
		return "-"
	}

	// Available colors with black background (avoiding red and green which are used for Ready status)
	colors := []*color.Color{
		color.New(color.FgBlue, color.BgBlack),
		color.New(color.FgMagenta, color.BgBlack),
		color.New(color.FgCyan, color.BgBlack),
		color.New(color.FgYellow, color.BgBlack),
		color.New(color.FgHiBlue, color.BgBlack),
		color.New(color.FgHiMagenta, color.BgBlack),
		color.New(color.FgHiCyan, color.BgBlack),
		color.New(color.FgHiYellow, color.BgBlack),
	}

	// Hash the app name to get consistent color assignment
	h := fnv.New32a()
	h.Write([]byte(appName))
	colorIndex := h.Sum32() % uint32(len(colors))

	return colors[colorIndex].Sprint(" " + appName + " ")
}

// FormatPRNumber formats PR numbers, using branch name for null values
func FormatPRNumber(prNumber, branchName string) string {
	if prNumber == "" || prNumber == "0" {
		// Create green background with yellow text for branch names
		branchStyle := color.New(color.BgGreen, color.FgBlack)
		return branchStyle.Sprint(" " + branchName + " ")
	}
	return prNumber
}

// FormatClickableUUID formats a UUID as a clickable link to shipyard.build details page
// Falls back to plain UUID if terminal doesn't support OSC 8
func FormatClickableUUID(uuid string) string {
	if uuid == "" {
		return ""
	}

	detailsURL := "https://shipyard.build/application/" + uuid + "/detail"

	if supportsOSC8() {
		// OSC 8 escape sequence: \033]8;;URL\033\\TEXT\033]8;;\033\\
		return "\033]8;;" + detailsURL + "\033\\" + uuid + "\033]8;;\033\\"
	}

	// Fallback: return plain UUID
	return uuid
}

// FormatClickableUUIDWithBackground formats a UUID with background color for duplicates
func FormatClickableUUIDWithBackground(uuid string, bgColor *color.Color) string {
	if uuid == "" {
		return ""
	}

	detailsURL := "https://shipyard.build/application/" + uuid + "/detail"
	
	var formattedUUID string
	if supportsOSC8() {
		// OSC 8 escape sequence: \033]8;;URL\033\\TEXT\033]8;;\033\\
		formattedUUID = "\033]8;;" + detailsURL + "\033\\" + uuid + "\033]8;;\033\\"
	} else {
		formattedUUID = uuid
	}

	if bgColor != nil {
		if supportsOSC8() {
			// For clickable links, apply color to the visible text only
			coloredUUID := bgColor.Sprint(uuid)
			return "\033]8;;" + detailsURL + "\033\\" + coloredUUID + "\033]8;;\033\\"
		} else {
			return bgColor.Sprint(formattedUUID)
		}
	}
	return formattedUUID
}

// GetDuplicateUUIDs identifies UUIDs that appear more than once in the final table
func GetDuplicateUUIDs(envs []types.Environment) map[string]bool {
	uuidCounts := make(map[string]int)
	
	// Count occurrences of each UUID based on how many times they'll appear in the table
	// Each environment UUID appears once per project in that environment
	for _, env := range envs {
		projectCount := len(env.Attributes.Projects)
		if projectCount == 0 {
			projectCount = 1 // Ensure at least one row per environment
		}
		uuidCounts[env.ID] += projectCount
	}
	
	// Identify duplicates (UUIDs that appear more than once in the table)
	duplicates := make(map[string]bool)
	for uuid, count := range uuidCounts {
		if count > 1 {
			duplicates[uuid] = true
		}
	}
	
	return duplicates
}

// GenerateDuplicateColors creates consistent background colors for duplicate UUIDs
func GenerateDuplicateColors(duplicateUUIDs map[string]bool) map[string]*color.Color {
	if len(duplicateUUIDs) == 0 {
		return nil
	}
	
	// Available background colors (avoiding red/green used for Ready status)
	backgroundColors := []color.Attribute{
		color.BgBlue,
		color.BgMagenta,
		color.BgCyan,
		color.BgYellow,
		color.BgHiBlue,
		color.BgHiMagenta,
		color.BgHiCyan,
		color.BgHiYellow,
	}
	
	// Create seeded random generator for consistent colors
	rand.Seed(time.Now().UnixNano())
	
	colorMap := make(map[string]*color.Color)
	colorIndex := 0
	
	for uuid := range duplicateUUIDs {
		// Use hash of UUID to get consistent color assignment
		h := fnv.New32a()
		h.Write([]byte(uuid))
		selectedColorIndex := int(h.Sum32()) % len(backgroundColors)
		
		c := color.New(color.FgBlack, backgroundColors[selectedColorIndex])
		// Force enable colors even if terminal detection fails
		c.EnableColor()
		colorMap[uuid] = c
		colorIndex = (colorIndex + 1) % len(backgroundColors)
	}
	
	return colorMap
}

// FormattedEnvironment takes an environment, extracts data from it, and prepares it
// to be in tabular format. If the environment value is nil, the program will panic.
func FormattedEnvironment(env *types.Environment) [][]string {
	data := make([][]string, 0, len(env.Attributes.Projects))

	for _, p := range env.Attributes.Projects {
		pr := strconv.Itoa(p.PullRequestNumber)

		data = append(data, []string{
			FormatColoredAppName(env.Attributes.Name),
			FormatClickableUUID(env.ID),
			FormatReadyStatus(env.Attributes.Ready),
			p.RepoName,
			FormatPRNumber(pr, p.Branch),
			FormatClickableURL(env.Attributes.URL),
		})
	}

	return data
}

// FormattedEnvironmentWithDuplicateColors takes an environment and duplicate color mapping,
// extracts data from it, and prepares it to be in tabular format with background colors for duplicate UUIDs.
func FormattedEnvironmentWithDuplicateColors(env *types.Environment, duplicateColors map[string]*color.Color) [][]string {
	data := make([][]string, 0, len(env.Attributes.Projects))

	for _, p := range env.Attributes.Projects {
		pr := strconv.Itoa(p.PullRequestNumber)
		
		// Get background color for UUID if it's a duplicate
		var bgColor *color.Color
		if duplicateColors != nil {
			bgColor = duplicateColors[env.ID]
		}

		data = append(data, []string{
			FormatColoredAppName(env.Attributes.Name),
			FormatClickableUUIDWithBackground(env.ID, bgColor),
			FormatReadyStatus(env.Attributes.Ready),
			p.RepoName,
			FormatPRNumber(pr, p.Branch),
			FormatClickableURL(env.Attributes.URL),
		})
	}

	return data
}
