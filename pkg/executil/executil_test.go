package executil

import (
	"bytes"
	"fmt"
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

func TestWrapCommand(t *testing.T) {
	tests := []struct {
		name         string
		cmdName      string
		args         []string
		dbusAddress  string
		dbusExists   bool
		expectedCmd  string
		expectedArgs []string
	}{
		{
			name:         "flatpak with dbus session set",
			cmdName:      "flatpak",
			args:         []string{"install", "app"},
			dbusAddress:  "unix:path=/run/user/1000/bus",
			dbusExists:   true,
			expectedCmd:  "flatpak",
			expectedArgs: []string{"install", "app"},
		},
		{
			name:         "flatpak without dbus session, dbus-run-session exists",
			cmdName:      "flatpak",
			args:         []string{"install", "app"},
			dbusAddress:  "",
			dbusExists:   true,
			expectedCmd:  "dbus-run-session",
			expectedArgs: []string{"--", "flatpak", "install", "app"},
		},
		{
			name:         "flatpak-builder without dbus session, dbus-run-session exists",
			cmdName:      "flatpak-builder",
			args:         []string{"--force-clean", "build", "manifest.json"},
			dbusAddress:  "",
			dbusExists:   true,
			expectedCmd:  "dbus-run-session",
			expectedArgs: []string{"--", "flatpak-builder", "--force-clean", "build", "manifest.json"},
		},
		{
			name:         "flatpak without dbus session, dbus-run-session missing",
			cmdName:      "flatpak",
			args:         []string{"install", "app"},
			dbusAddress:  "",
			dbusExists:   false,
			expectedCmd:  "flatpak",
			expectedArgs: []string{"install", "app"},
		},
		{
			name:         "unrelated command",
			cmdName:      "git",
			args:         []string{"status"},
			dbusAddress:  "",
			dbusExists:   true,
			expectedCmd:  "git",
			expectedArgs: []string{"status"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookPath := func(file string) (string, error) {
				if file == "dbus-run-session" && tc.dbusExists {
					return "/usr/bin/dbus-run-session", nil
				}
				return "", fmt.Errorf("not found")
			}
			getenv := func(key string) string {
				if key == "DBUS_SESSION_BUS_ADDRESS" {
					return tc.dbusAddress
				}
				return ""
			}

			cmdName, cmdArgs := wrapCommand(lookPath, getenv, tc.cmdName, tc.args...)
			if cmdName != tc.expectedCmd {
				t.Errorf("expected cmd %q, got %q", tc.expectedCmd, cmdName)
			}
			if len(cmdArgs) != len(tc.expectedArgs) {
				t.Fatalf("expected args length %d, got %d", len(tc.expectedArgs), len(cmdArgs))
			}
			for i, arg := range cmdArgs {
				if arg != tc.expectedArgs[i] {
					t.Errorf("at index %d: expected arg %q, got %q", i, tc.expectedArgs[i], arg)
				}
			}
		})
	}
}
