package logger

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestInitPlain(t *testing.T) {
	// Initialize with plain mode active
	Init(true, false, true)
	if !IsPlain() {
		t.Error("expected IsPlain() to return true")
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Output success banner
	SuccessBanner("Done", "Operation finished successfully.")

	// Restore stdout and read output
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "✔  Done") {
		t.Errorf("expected plain success banner output to contain '✔  Done', got: %q", output)
	}
	if !strings.Contains(output, "Operation finished successfully.") {
		t.Errorf("expected plain success banner output to contain the message, got: %q", output)
	}
	// Check that it does not contain ANSI escape codes
	if strings.Contains(output, "\u001b") {
		t.Errorf("expected plain success banner to not contain ANSI escape codes, got: %q", output)
	}

	// Reset state for subsequent tests
	Init(false, false, false)
}

func TestInitNonPlain(t *testing.T) {
	// Initialize with plain mode inactive
	Init(false, false, false)
	if IsPlain() {
		t.Error("expected IsPlain() to return false")
	}
}
