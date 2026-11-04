package tools

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// docs/mcp.md lists the tools by kind and gives Kiro users a disabledTools
// block-list. Both are copied by hand from the annotations map, so a new or
// reclassified tool must update the doc too: a tool missing from the Kiro list
// runs unattended without asking.
func TestDocsMatchAnnotations(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/mcp.md")
	if err != nil {
		t.Fatalf("reading docs/mcp.md: %v", err)
	}
	doc := string(raw)

	var readOnly, destructive, additive []string
	for name, a := range annotations {
		switch {
		case a.ReadOnlyHint:
			readOnly = append(readOnly, name)
		case a.DestructiveHint:
			destructive = append(destructive, name)
		default:
			additive = append(additive, name)
		}
	}

	backticked := regexp.MustCompile("`([a-z_]+)`")
	for _, row := range []struct {
		prefix string
		want   []string
	}{
		{"| Read-only |", readOnly},
		{"| Destructive", destructive},
		{"| Adds state only |", additive},
	} {
		line := lineWithPrefix(doc, row.prefix)
		if line == "" {
			t.Errorf("docs/mcp.md has no table row starting %q", row.prefix)
			continue
		}
		assertSameTools(t, "row "+row.prefix, matches(backticked, line), row.want)
	}

	start := strings.Index(doc, `"disabledTools": [`)
	if start < 0 {
		t.Fatal(`docs/mcp.md has no Kiro "disabledTools" list`)
	}
	list := doc[start : start+strings.Index(doc[start:], "]")]
	quoted := regexp.MustCompile(`"([a-z_]+)"`)
	assertSameTools(t, "Kiro disabledTools", matches(quoted, list), destructive)
}

func lineWithPrefix(doc, prefix string) string {
	for line := range strings.SplitSeq(doc, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func matches(re *regexp.Regexp, s string) []string {
	var names []string
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		if m[1] != "disabledTools" {
			names = append(names, m[1])
		}
	}
	return names
}

func assertSameTools(t *testing.T, where string, got, want []string) {
	t.Helper()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("docs/mcp.md %s lists %v, annotations say %v", where, got, want)
	}
}
