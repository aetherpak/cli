package configdiff

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestUnifiedPlainShowsAddedLines(t *testing.T) {
	oldB := []byte("apps:\n  - id: a\n")
	newB := []byte("apps:\n  - id: a\n  - id: b\n")
	got := Unified(oldB, newB, "aetherpak.yaml", true /* plain */)
	if !strings.Contains(got, "+") || !strings.Contains(got, "id: b") {
		t.Errorf("diff missing added line:\n%s", got)
	}
	// No ANSI escapes in plain mode.
	if strings.Contains(got, "\x1b[") {
		t.Errorf("plain diff contains ANSI escapes:\n%q", got)
	}
}

func TestUnifiedNoChange(t *testing.T) {
	b := []byte("apps:\n  - id: a\n")
	if got := Unified(b, b, "aetherpak.yaml", true); strings.TrimSpace(got) != "" {
		t.Errorf("expected empty diff, got:\n%s", got)
	}
}

func TestUnifiedStyledWithAdditionsAndDeletions(t *testing.T) {
	// Force Lip Gloss to render styles using TrueColor so ANSI escape codes are emitted.
	originalProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(originalProfile)

	oldB := []byte("apps:\n  - id: a\n  - id: b\n")
	newB := []byte("apps:\n  - id: a\n  - id: c\n")

	// Test styled output
	got := Unified(oldB, newB, "aetherpak.yaml", false /* plain */)
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("expected styled diff to contain ANSI escapes, got: %q", got)
	}
	if !strings.Contains(got, "id: b") {
		t.Errorf("expected diff to contain deletion line info, got: %q", got)
	}
	if !strings.Contains(got, "id: c") {
		t.Errorf("expected diff to contain addition line info, got: %q", got)
	}
}
