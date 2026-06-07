package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectRepoCmd(t *testing.T) {
	// Create a temp repository with a fake ref to inspect
	tmp := t.TempDir()
	headsDir := filepath.Join(tmp, "refs", "heads")
	refPath := filepath.Join(headsDir, "app", "org.example.InspectTest", "x86_64", "stable")
	if err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {
		t.Fatalf("failed to create temp dirs: %v", err)
	}
	if err := os.WriteFile(refPath, []byte("dummy-commit-sha"), 0644); err != nil {
		t.Fatalf("failed to write mock ref file: %v", err)
	}

	inspectRepoPath = tmp
	inspectOutputFile = ""

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := inspectRepoCmd.RunE(inspectRepoCmd, nil)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("inspectRepoCmd.RunE failed: %v", err)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "app-id=org.example.InspectTest") {
		t.Errorf("expected output to contain app-id=org.example.InspectTest, got: %q", output)
	}
	if !strings.Contains(output, "arch=x86_64") {
		t.Errorf("expected output to contain arch=x86_64, got: %q", output)
	}
	if !strings.Contains(output, "branch=stable") {
		t.Errorf("expected output to contain branch=stable, got: %q", output)
	}
}

func TestInspectRepoCmdError(t *testing.T) {
	// Use non-existent path to trigger resolution error
	inspectRepoPath = "/non/existent/path/repo"
	inspectOutputFile = ""

	err := inspectRepoCmd.RunE(inspectRepoCmd, nil)
	if err == nil {
		t.Error("expected error when resolving non-existent repo path, got nil")
	}
}
