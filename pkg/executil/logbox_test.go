package executil

import (
	"bytes"
	"strings"
	"testing"
)

func TestLogBox(t *testing.T) {
	var buf bytes.Buffer
	lb := NewLogBox(&buf, 3)

	lb.Start()
	// Box should not draw initially (lazy rendering)
	initialStr := buf.String()
	if initialStr != "" {
		t.Errorf("expected no initial box rendering, got: %q", initialStr)
	}

	_, err := lb.Write([]byte("flatpak-builder │ first line\n"))
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	render1 := buf.String()
	if !strings.Contains(render1, "first line") {
		t.Errorf("expected log line in rendering, got: %q", render1)
	}

	// Write more lines to trigger scroll/cache rotation
	buf.Reset()
	lb.Write([]byte("flatpak-builder │ second line\n"))
	lb.Write([]byte("flatpak-builder │ third line\n"))
	buf.Reset() // Reset buffer so we only capture the final frame
	lb.Write([]byte("flatpak-builder │ fourth line\n"))

	render4 := buf.String()
	// Since maxLines = 3, "first line" should be rolled out
	if strings.Contains(render4, "first line") {
		t.Errorf("expected 'first line' to be rolled out of box, got: %q", render4)
	}
	if !strings.Contains(render4, "fourth line") {
		t.Errorf("expected 'fourth line' to be present, got: %q", render4)
	}

	// Check Close flushes partial buffer
	buf.Reset()
	lb.Write([]byte("flatpak-builder │ final partial"))
	lb.Close()
	renderFinal := buf.String()
	if !strings.Contains(renderFinal, "final partial") {
		t.Errorf("expected final partial line to be flushed on Close, got: %q", renderFinal)
	}
}
