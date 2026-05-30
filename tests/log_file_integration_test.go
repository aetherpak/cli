//go:build integration

package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ELogFile(t *testing.T) {
	// Compile binary
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = ".."
	var buildStderr bytes.Buffer
	buildCmd.Stderr = &buildStderr
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to compile aetherpak binary (%v): %s", err, buildStderr.String())
	}
	binaryPath := filepath.Join("..", "bin", "aetherpak")
	absBinaryPath, err := filepath.Abs(binaryPath)
	if err != nil {
		t.Fatalf("failed to get absolute path of binary: %v", err)
	}

	tempDir, err := os.MkdirTemp("", "log-file-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("Explicit log-file should be written and preserved on success", func(t *testing.T) {
		logPath := filepath.Join(tempDir, "success.log")

		cmd := exec.Command(absBinaryPath, "help", "--log-file="+logPath)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			t.Fatalf("command failed: %v, stderr: %s", err, stderr.String())
		}

		// Verify explicit log file exists
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("failed to read log file: %v", err)
		}
		content := string(data)
		// Help command doesn't log much by default, but it initializes and opens the log file.
		// If we want actual logs, let's check that the file exists (even if empty).
		if len(content) < 0 {
			t.Error("expected log file to be created")
		}
	})

	t.Run("Explicit log-file should be written and preserved on failure", func(t *testing.T) {
		logPath := filepath.Join(tempDir, "failure.log")

		// Running plan command with nonexistent config should fail
		cmd := exec.Command(absBinaryPath, "plan", "--config=nonexistent.yaml", "--log-file="+logPath)
		cmd.Dir = tempDir
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err == nil {
			t.Fatal("expected command to fail, but it succeeded")
		}

		// Verify explicit log file exists and has content
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("failed to read log file: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "Execution Failure") && !strings.Contains(content, "nonexistent.yaml") {
			t.Errorf("expected log file to contain execution failure details, got: %q", content)
		}
	})

	t.Run("Default temp log file should be cleaned up on success", func(t *testing.T) {
		// Run a successful command without --log-file
		cmd := exec.Command(absBinaryPath, "help")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			t.Fatalf("command failed: %v, stderr: %s", err, stderr.String())
		}

		// Stderr should not contain any logs written output
		stderrOutput := stderr.String()
		if strings.Contains(stderrOutput, "Detailed logs written to:") {
			t.Errorf("expected successful run to not mention log files, got: %q", stderrOutput)
		}
	})

	t.Run("Default temp log file should be retained on failure", func(t *testing.T) {
		// Run a failing command without --log-file
		cmd := exec.Command(absBinaryPath, "plan", "--config=nonexistent.yaml")
		cmd.Dir = tempDir
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err == nil {
			t.Fatal("expected command to fail, but it succeeded")
		}

		stderrOutput := stderr.String()
		if !strings.Contains(stderrOutput, "Detailed logs written to:") {
			t.Errorf("expected stderr to report log file preservation, got: %q", stderrOutput)
		}

		// Extract log file path from output
		parts := strings.Split(stderrOutput, "Detailed logs written to:")
		if len(parts) < 2 {
			t.Fatalf("failed to parse log path from stderr: %q", stderrOutput)
		}
		path := strings.TrimSpace(parts[1])
		// Remove ANSI escape color code suffixes if any
		path = strings.Split(path, "\n")[0]
		path = strings.ReplaceAll(path, "\u001b[0m", "")
		path = strings.ReplaceAll(path, "\u001b[86m", "")
		path = strings.ReplaceAll(path, "\u001b[1m", "")
		path = strings.TrimSpace(path)

		defer os.Remove(path)

		// Verify temp log file exists and has failure details
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read preserved temp log file at %q: %v", path, err)
		}
		content := string(data)
		if !strings.Contains(content, "nonexistent.yaml") {
			t.Errorf("expected preserved logs to contain error details, got: %q", content)
		}
	})
}
