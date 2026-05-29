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

func TestE2EConfigOverrides(t *testing.T) {
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = ".."
	var buildStderr bytes.Buffer
	buildCmd.Stderr = &buildStderr
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to compile aetherpak binary (%v): %s", err, buildStderr.String())
	}
	binaryPath := filepath.Join("..", "bin", "aetherpak")

	configData := []byte(`
registry: registry.example.com
remote_name: custom-remote
channel_mappings:
  "main": "beta"
  "staging/*": "alpha"
  "release-*": "stable"
`)

	err := os.WriteFile("aetherpak.yaml", configData, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	tests := []struct {
		name     string
		refType  string
		refName  string
		expected string
	}{
		{
			name:     "main branch to beta",
			refType:  "branch",
			refName:  "main",
			expected: "beta",
		},
		{
			name:     "staging wildcard branch to alpha",
			refType:  "branch",
			refName:  "staging/test-run",
			expected: "alpha",
		},
		{
			name:     "release wildcard branch to stable",
			refType:  "branch",
			refName:  "release-1.2",
			expected: "stable",
		},
		{
			name:     "tag to stable default",
			refType:  "tag",
			refName:  "v1.0.0",
			expected: "stable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binaryPath, "resolve-channel",
				"--ref-type="+tt.refType,
				"--ref-name="+tt.refName,
			)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("resolve-channel failed: %v, stderr: %s", err, stderr.String())
			}
			actual := strings.TrimSpace(stdout.String())
			if actual != tt.expected {
				t.Errorf("expected channel %q, got %q", tt.expected, actual)
			}
		})
	}

	t.Run("env variable override registry", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "resolve-channel",
			"--ref-type=branch",
			"--ref-name=main",
		)
		cmd.Env = append(os.Environ(), "AETHERPAK_CHANNEL_MAPPINGS_MAIN=custom-channel")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("resolve-channel failed with env override: %v, stderr: %s", err, stderr.String())
		}
		actual := strings.TrimSpace(stdout.String())
		if actual != "custom-channel" {
			t.Errorf("expected env overridden channel %q, got %q", "custom-channel", actual)
		}
	})
}
