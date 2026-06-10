package executil

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamWithPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		prefix   string
		expected string
	}{
		{
			name:     "regular lines",
			input:    "line1\nline2\n",
			prefix:   "pref |",
			expected: "pref | line1\npref | line2\n",
		},
		{
			name:     "carriage returns",
			input:    "line1\r\nline2\r\n",
			prefix:   "pref |",
			expected: "pref | line1\npref | line2\n",
		},
		{
			name:     "no trailing newline",
			input:    "line1\nline2",
			prefix:   "pref |",
			expected: "pref | line1\npref | line2\n",
		},
		{
			name:     "empty input",
			input:    "",
			prefix:   "pref |",
			expected: "",
		},
		{
			name:     "empty prefix",
			input:    "line1\nline2\n",
			prefix:   "",
			expected: "line1\nline2\n",
		},
		{
			name:     "long line",
			input:    strings.Repeat("a", 10000) + "\n",
			prefix:   "pref |",
			expected: "pref | " + strings.Repeat("a", 10000) + "\n",
		},
		{
			name:     "long line no newline",
			input:    strings.Repeat("b", 10000),
			prefix:   "pref |",
			expected: "pref | " + strings.Repeat("b", 10000) + "\n",
		},
		{
			name:     "extremely long line triggers flush at 64KB",
			input:    strings.Repeat("c", 70000),
			prefix:   "pref |",
			expected: "pref | " + strings.Repeat("c", 65536) + "\npref | " + strings.Repeat("c", 4464) + "\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			StreamWithPrefix(strings.NewReader(tc.input), &buf, tc.prefix)
			actual := buf.String()
			if actual != tc.expected {
				t.Errorf("expected:\n%q\ngot:\n%q", tc.expected, actual)
			}
		})
	}
}

func TestStreamToTargets(t *testing.T) {
	input := "hello\nworld\r\nfinal line"
	var buf1, buf2 bytes.Buffer

	targets := []StreamTarget{
		{Writer: &buf1, Prefix: "W1 |"},
		{Writer: &buf2, Prefix: "W2 |"},
	}

	StreamToTargets(strings.NewReader(input), targets...)

	expected1 := "W1 | hello\nW1 | world\nW1 | final line\n"
	expected2 := "W2 | hello\nW2 | world\nW2 | final line\n"

	if actual1 := buf1.String(); actual1 != expected1 {
		t.Errorf("buf1 mismatch:\nexpected: %q\ngot: %q", expected1, actual1)
	}
	if actual2 := buf2.String(); actual2 != expected2 {
		t.Errorf("buf2 mismatch:\nexpected: %q\ngot: %q", expected2, actual2)
	}
}

func TestOSCommand_DbusLifecycle(t *testing.T) {
	// Backup hooks
	origStartDbus := startDbusSessionFunc
	origTerminate := terminateProcess
	defer func() {
		startDbusSessionFunc = origStartDbus
		terminateProcess = origTerminate
	}()

	// Clear DBUS_SESSION_BUS_ADDRESS temporarily to force transient D-Bus start
	origDbusAddress := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	os.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	defer os.Setenv("DBUS_SESSION_BUS_ADDRESS", origDbusAddress)

	var dbusStarted bool
	var terminatedPid int
	var terminateCalled bool

	startDbusSessionFunc = func(lookPath func(string) (string, error)) (string, int, error) {
		dbusStarted = true
		return "unix:path=/tmp/mock-dbus", 99999, nil
	}

	terminateProcess = func(pid int) error {
		terminateCalled = true
		terminatedPid = pid
		return nil
	}

	tmpDir := t.TempDir()
	mockFlatpakPath := filepath.Join(tmpDir, "flatpak")
	// Write a simple shell script
	err := os.WriteFile(mockFlatpakPath, []byte("#!/bin/sh\nexit 0\n"), 0755)
	if err != nil {
		t.Fatalf("failed to write mock flatpak script: %v", err)
	}

	// 1. Test successful command Start and Wait
	execCmd := exec.Command(mockFlatpakPath)
	c := &osCommand{cmd: execCmd}

	dbusStarted = false
	terminateCalled = false
	terminatedPid = 0

	err = c.Start()
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if !dbusStarted {
		t.Error("expected D-Bus session to be started")
	}

	if c.dbusPID != 99999 {
		t.Errorf("expected dbusPID to be 99999, got %d", c.dbusPID)
	}

	// Check if DBUS_SESSION_BUS_ADDRESS env was set
	var dbusEnvVal string
	for _, env := range c.cmd.Env {
		if strings.HasPrefix(env, "DBUS_SESSION_BUS_ADDRESS=") {
			dbusEnvVal = env
		}
	}
	if dbusEnvVal == "" {
		t.Error("expected DBUS_SESSION_BUS_ADDRESS to be added to command Env")
	} else if dbusEnvVal != "DBUS_SESSION_BUS_ADDRESS=unix:path=/tmp/mock-dbus" {
		t.Errorf("unexpected DBUS env value: %s", dbusEnvVal)
	}

	err = c.Wait()
	if err != nil {
		t.Fatalf("Wait() failed: %v", err)
	}

	if !terminateCalled {
		t.Error("expected terminateProcess to be called on Wait()")
	}
	if terminatedPid != 99999 {
		t.Errorf("expected terminateProcess called with PID 99999, got %d", terminatedPid)
	}
	if c.dbusPID != 0 {
		t.Errorf("expected dbusPID to be reset to 0 after Wait(), got %d", c.dbusPID)
	}

	// 2. Test Start failure cleanup
	dbusStarted = false
	terminateCalled = false
	terminatedPid = 0

	// Use an invalid path that ends in flatpak to trigger Start failure
	execCmdFail := exec.Command(filepath.Join(tmpDir, "nonexistent-flatpak"))
	cFail := &osCommand{cmd: execCmdFail}

	err = cFail.Start()
	if err == nil {
		t.Error("expected Start() to fail for nonexistent command")
	}

	if !dbusStarted {
		t.Error("expected D-Bus session to be started before cmd Start()")
	}

	if !terminateCalled {
		t.Error("expected terminateProcess to be called on Start() failure")
	}
	if terminatedPid != 99999 {
		t.Errorf("expected terminateProcess called with PID 99999, got %d", terminatedPid)
	}
	if cFail.dbusPID != 0 {
		t.Errorf("expected dbusPID to be reset to 0 after Start() failure, got %d", cFail.dbusPID)
	}
}
