// Package configdiff renders a colored unified diff between two config blobs.
package configdiff

import (
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/charmbracelet/lipgloss"
)

var (
	addStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // green
	delStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203")) // red
	hdrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))  // blue
)

// Unified returns a unified diff of oldB vs newB labeled with filename. When
// plain is true, no ANSI styling is applied (for non-TTY / --plain output and
// deterministic tests). It returns the empty string when there is no change.
func Unified(oldB, newB []byte, filename string, plain bool) string {
	diff := udiff.Unified("a/"+filename, "b/"+filename, string(oldB), string(newB))
	if diff == "" {
		return ""
	}
	if plain {
		return diff
	}
	var b strings.Builder
	// Drop the trailing newline so styling doesn't add an empty final line.
	for _, line := range strings.Split(strings.TrimRight(diff, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"), strings.HasPrefix(line, "@@"):
			b.WriteString(hdrStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			b.WriteString(addStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			b.WriteString(delStyle.Render(line))
		default:
			b.WriteString(line)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
