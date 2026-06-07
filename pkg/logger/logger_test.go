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
	if ci := os.Getenv("CI"); ci != "" {
		os.Unsetenv("CI")
		defer os.Setenv("CI", ci)
	}

	// Initialize with plain mode inactive
	Init(false, false, false)
	if IsPlain() {
		t.Error("expected IsPlain() to return false")
	}
}

func TestTempDir(t *testing.T) {
	origXdg := os.Getenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", origXdg)

	dummyDir, err := os.MkdirTemp("", "aetherpak-xdg-test-*")
	if err != nil {
		t.Fatalf("failed to create dummy dir: %v", err)
	}
	defer os.RemoveAll(dummyDir)

	os.Setenv("XDG_RUNTIME_DIR", dummyDir)
	if td := TempDir(); td != dummyDir {
		t.Errorf("expected TempDir to be %q, got %q", dummyDir, td)
	}

	nonExistent := dummyDir + "-non-existent"
	os.Setenv("XDG_RUNTIME_DIR", nonExistent)
	if td := TempDir(); td == nonExistent {
		t.Errorf("expected TempDir to fallback from non-existent directory %q", nonExistent)
	}

	os.Unsetenv("XDG_RUNTIME_DIR")
	if td := TempDir(); td != os.TempDir() {
		t.Errorf("expected TempDir to be %q when XDG_RUNTIME_DIR is unset, got %q", os.TempDir(), td)
	}
}

func TestInitFileLoggingExplicitPath(t *testing.T) {
	tempFile, err := os.CreateTemp(TempDir(), "logger-test-*.log")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	// Initialize file logging to explicit path
	err = InitFileLogging(tempPath)
	if err != nil {
		t.Fatalf("failed to initialize file logging: %v", err)
	}

	Info("test info message explicit")
	Error("test error message explicit")
	SuccessBanner("DoneExplicit", "Finished explicit")

	// Close the log file, asserting no error
	CloseLogFile(false)

	// Verify log file still exists
	data, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "test info message explicit") {
		t.Errorf("expected logs to contain info message, got %q", content)
	}
	if !strings.Contains(content, "test error message explicit") {
		t.Errorf("expected logs to contain error message, got %q", content)
	}
	if !strings.Contains(content, "DoneExplicit") {
		t.Errorf("expected logs to contain success banner, got %q", content)
	}
}

func TestInitFileLoggingTempSuccess(t *testing.T) {
	// Initialize file logging with empty string (creates temporary log file)
	err := InitFileLogging("")
	if err != nil {
		t.Fatalf("failed to initialize temporary file logging: %v", err)
	}

	path := logFilePath
	if path == "" {
		t.Fatal("expected logFilePath to be set, got empty")
	}
	if !isTempLogFile {
		t.Error("expected isTempLogFile to be true")
	}

	Info("test info message temp success")

	// Verify temp file exists and has content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read temp log file: %v", err)
	}
	if !strings.Contains(string(data), "test info message temp success") {
		t.Errorf("expected temp logs to contain message, got %q", string(data))
	}

	// Close with hasError=false (success)
	CloseLogFile(false)

	// Verify temp file is cleaned up (deleted)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected temp file to be deleted, but it still exists at %q", path)
	}
}

func TestInitFileLoggingTempFailure(t *testing.T) {
	// Initialize file logging with empty string (creates temporary log file)
	err := InitFileLogging("")
	if err != nil {
		t.Fatalf("failed to initialize temporary file logging: %v", err)
	}

	path := logFilePath
	if path == "" {
		t.Fatal("expected logFilePath to be set, got empty")
	}
	defer os.Remove(path)

	Info("test info message temp failure")

	// Capture stderr to check for output message
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Close with hasError=true (failure)
	CloseLogFile(true)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	stderrOutput := buf.String()

	// Verify temp file still exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected temp log file to be retained on failure, but it was deleted")
	}

	// Verify notification written to stderr
	if !strings.Contains(stderrOutput, "Detailed logs written to:") {
		t.Errorf("expected stderr to contain warning message, got %q", stderrOutput)
	}
}

func TestErrorBanner(t *testing.T) {
	// Initialize with plain mode active
	Init(true, false, true)
	if !IsPlain() {
		t.Error("expected IsPlain() to return true")
	}

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Output error banner
	ErrorBanner("Failed", "An error occurred.")

	// Restore stderr and read output
	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Failed") || !strings.Contains(output, "An error occurred.") {
		t.Errorf("expected plain error banner output to contain 'Failed' and message, got: %q", output)
	}

	// Test non-plain style (no crash/runs successfully)
	Init(false, false, false)
	ErrorBanner("FailedNonPlain", "Styled error.")
}

func TestLogLevels(t *testing.T) {
	// Verbose initialization to ensure debug logs are printed
	Init(true, false, true)

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	appLogger.SetOutput(w)

	// Log at different levels
	Log(LevelDebug, "debug message %s", "1")
	Log(LevelWarn, "warn message %s", "2")
	Log(LevelError, "error message %s", "3")
	Log(LogLevel("unknown"), "info message %s", "4") // should trigger Info case

	Warn("direct warn %s", "5")
	Debug("direct debug %s", "6")

	// Restore stderr and read output
	w.Close()
	os.Stderr = oldStderr
	appLogger.SetOutput(os.Stderr)

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	expectedMessages := []string{
		"debug message 1",
		"warn message 2",
		"error message 3",
		"info message 4",
		"direct warn 5",
		"direct debug 6",
	}

	for _, msg := range expectedMessages {
		if !strings.Contains(output, msg) {
			t.Errorf("expected log output to contain %q, got: %q", msg, output)
		}
	}
}
