package executil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// LogBox implements io.WriteCloser to display log lines inside a scrolling, fixed-height boxed container.
type LogBox struct {
	mu          sync.Mutex
	lines       []string
	maxLines    int
	initialized bool
	writer      io.Writer
	buf         bytes.Buffer
}

// NewLogBox creates a new LogBox instance.
func NewLogBox(writer io.Writer, maxLines int) *LogBox {
	return &LogBox{
		writer:   writer,
		maxLines: maxLines,
		lines:    make([]string, 0, maxLines),
	}
}

// Start prints the initial layout of the LogBox.
func (lb *LogBox) Start() {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Disable line wrapping
	fmt.Fprint(lb.writer, "\033[?7l")

	lb.redrawLocked()
	lb.initialized = true
}

func (lb *LogBox) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	n = len(p)
	lb.buf.Write(p)

	for {
		lineBytes, err := lb.buf.ReadBytes('\n')
		if err != nil {
			// No complete line yet, restore buffer and stop
			lb.buf.Write(lineBytes)
			break
		}

		line := string(bytes.TrimSuffix(lineBytes, []byte("\n")))
		lb.addLineLocked(line)
	}

	return n, nil
}

// Close flushes any remaining buffered text and finalizes the LogBox.
func (lb *LogBox) Close() error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if lb.buf.Len() > 0 {
		line := lb.buf.String()
		lb.addLineLocked(line)
		lb.buf.Reset()
	}

	// Re-enable line wrapping
	fmt.Fprint(lb.writer, "\033[?7h")
	return nil
}

func (lb *LogBox) addLineLocked(rawLine string) {
	clean := ansiRegex.ReplaceAllString(rawLine, "")

	// Query terminal width to know the limits
	width, _, err := term.GetSize(uintptr(os.Stdout.Fd()))
	if err != nil || width < 40 {
		width = 80
	}
	if width > 100 {
		width = 100 // Visual balance cap
	}

	// Identify stream types
	isStderr := strings.Contains(rawLine, "203")
	isLinter := strings.Contains(rawLine, "13") || strings.Contains(clean, "flatpak-builder-lint")

	formatted := lb.formatLine(clean, isStderr, isLinter, width-4)

	lb.lines = append(lb.lines, formatted)
	if len(lb.lines) > lb.maxLines {
		lb.lines = lb.lines[1:]
	}

	lb.redrawLocked()
}

func (lb *LogBox) formatLine(clean string, isStderr, isLinter bool, limit int) string {
	prefixText := "flatpak-builder"
	if isLinter {
		prefixText = "flatpak-builder-lint"
	}

	prefixWidth := len(prefixText) + 3
	msgLimit := limit - prefixWidth
	if msgLimit < 10 {
		msgLimit = 10
	}

	var msg string
	parts := strings.SplitN(clean, " │ ", 2)
	if len(parts) == 2 {
		msg = strings.TrimSpace(parts[1])
	} else {
		// Fallback for cases without the " │ " delimiter
		msg = strings.TrimSpace(clean)
		if strings.HasPrefix(msg, prefixText+" |") {
			msg = strings.TrimSpace(strings.TrimPrefix(msg, prefixText+" |"))
		}
	}

	if len(msg) > msgLimit {
		msg = msg[:msgLimit-3] + "..."
	}

	var styledPrefix string
	if isLinter {
		styledPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true).Render(prefixText) +
			lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(" │")
	} else if isStderr {
		styledPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true).Render(prefixText) +
			lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(" │")
	} else {
		styledPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).Render(prefixText) +
			lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(" │")
	}

	return styledPrefix + " " + msg
}

func (lb *LogBox) redrawLocked() {
	width, _, err := term.GetSize(uintptr(os.Stdout.Fd()))
	if err != nil || width < 40 {
		width = 80
	}
	if width > 100 {
		width = 100
	}

	// Move cursor up if we previously drew a box
	if lb.initialized {
		fmt.Fprintf(lb.writer, "\r\033[%dA", lb.maxLines+2)
	}

	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true)

	title := " flatpak-builder logs "
	titleLen := len(title)
	borderWidth := width - titleLen - 4
	if borderWidth < 2 {
		borderWidth = 2
	}
	topBorder := borderStyle.Render("┌──") + titleStyle.Render(title) + borderStyle.Render(strings.Repeat("─", borderWidth)+"┐")
	fmt.Fprintln(lb.writer, topBorder)

	for i := 0; i < lb.maxLines; i++ {
		var content string
		if i < len(lb.lines) {
			content = lb.lines[i]
		}

		visWidth := lipgloss.Width(content)
		limit := width - 4
		var paddedContent string
		if visWidth > limit {
			paddedContent = content[:limit] // Should not happen since we truncated in formatLine
		} else {
			paddedContent = content + strings.Repeat(" ", limit-visWidth)
		}

		fmt.Fprintf(lb.writer, "%s %s %s\n", borderStyle.Render("│"), paddedContent, borderStyle.Render("│"))
	}

	bottomBorder := borderStyle.Render("└" + strings.Repeat("─", width-2) + "┘")
	fmt.Fprintln(lb.writer, bottomBorder)
}
