package selfupdate

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/fatih/color"
)

// DefaultNotesLines caps how much of the release notes is printed after an
// upgrade, so a jump across several releases doesn't flood the terminal.
const DefaultNotesLines = 40

var (
	linkRe   = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	boldRe   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	codeRe   = regexp.MustCompile("`([^`]+)`")
	bulletRe = regexp.MustCompile(`^(\s*)[-*] (.*)$`)
	// headingRe needs the space after the hashes, so "#123 fixed" stays text.
	headingRe = regexp.MustCompile(`^#{1,6} +(.*)$`)
)

// RenderNotes prints the releases' notes, newest first, as terminal text:
// headings in bold, bullets as "•", code in cyan, links as their text. Output
// stops after maxLines with a pointer to the full notes.
func RenderNotes(w io.Writer, releases []Release, maxLines int) {
	title := color.New(color.Bold, color.FgHiGreen)
	heading := color.New(color.Bold)
	code := color.New(color.FgCyan)
	dim := color.New(color.Faint)

	var lines []string
	for i, r := range releases {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, title.Sprintf("What's new in %s", r.Version()))
		body := strings.TrimSpace(strings.ReplaceAll(r.Body, "\r\n", "\n"))
		if body == "" {
			lines = append(lines, dim.Sprintf("  No release notes. See %s", r.HTMLURL))
			continue
		}
		blank := false
		for _, l := range strings.Split(body, "\n") {
			l = strings.TrimRight(l, " \t")
			if l == "" {
				if !blank {
					lines = append(lines, "")
				}
				blank = true
				continue
			}
			blank = false
			lines = append(lines, renderLine(l, heading, code))
		}
	}

	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
		more := "See the full release notes"
		if len(releases) > 0 && releases[0].HTMLURL != "" {
			more += ": " + releases[0].HTMLURL
		}
		lines = append(lines, "", dim.Sprint("  … "+more))
	}
	for _, l := range lines {
		_, _ = fmt.Fprintln(w, l)
	}
}

func renderLine(l string, heading, code *color.Color) string {
	if m := headingRe.FindStringSubmatch(l); m != nil {
		l = strings.TrimSpace(m[1])
		l = linkRe.ReplaceAllString(boldRe.ReplaceAllString(l, "$1"), "$1")
		return heading.Sprint(strings.ReplaceAll(l, "`", ""))
	}
	indent := "  "
	if m := bulletRe.FindStringSubmatch(l); m != nil {
		indent += m[1] + "• "
		l = m[2]
	}
	l = linkRe.ReplaceAllString(l, "$1")
	l = boldRe.ReplaceAllString(l, "$1")
	l = codeRe.ReplaceAllStringFunc(l, func(s string) string { return code.Sprint(strings.Trim(s, "`")) })
	return indent + l
}
