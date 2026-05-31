package configdiff

import (
	"strings"
	"testing"
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
