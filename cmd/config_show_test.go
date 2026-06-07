package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/spf13/viper"
)

func TestConfigShow(t *testing.T) {
	// Create mock config file
	data := []byte(`
registry: ghcr.io
remote_name: custom-remote
defaults:
  ccache: true
`)
	err := os.WriteFile("aetherpak.yaml", data, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	// Ensure Viper is reset before loading
	viper.Reset()
	initConfig()

	// 1. Run in plain mode
	logger.Init(false, false, true) // force plain mode
	buf := new(bytes.Buffer)
	configShowCmd.SetOut(buf)
	err = configShowCmd.RunE(configShowCmd, []string{})
	if err != nil {
		t.Fatalf("failed to run config show: %v", err)
	}
	output := buf.String()

	if !strings.Contains(output, "registry: ghcr.io") {
		t.Errorf("expected output to contain 'registry: ghcr.io', got %q", output)
	}
	if !strings.Contains(output, "remote_name: custom-remote") {
		t.Errorf("expected output to contain 'remote_name: custom-remote', got %q", output)
	}
	if !strings.Contains(output, "ccache: true") {
		t.Errorf("expected output to contain 'ccache: true', got %q", output)
	}
	if !strings.Contains(output, "--- Resolved Configuration ---") {
		t.Errorf("expected output to contain plain text header, got %q", output)
	}

	// 2. Set an environment variable override and test active overrides listing
	os.Setenv("AETHERPAK_REGISTRY", "quay.io")
	defer os.Unsetenv("AETHERPAK_REGISTRY")

	viper.Reset()
	initConfig()

	buf.Reset()
	err = configShowCmd.RunE(configShowCmd, []string{})
	if err != nil {
		t.Fatalf("failed to run config show with env override: %v", err)
	}
	output = buf.String()

	if !strings.Contains(output, "registry: quay.io") {
		t.Errorf("expected registry to be resolved to 'quay.io', got %q", output)
	}
	if !strings.Contains(output, "- Environment: AETHERPAK_REGISTRY=quay.io") {
		t.Errorf("expected active overrides listing to contain 'AETHERPAK_REGISTRY=quay.io', got %q", output)
	}
}

func TestConfigShowRich(t *testing.T) {
	// Create mock config file
	data := []byte(`
registry: ghcr.io
remote_name: custom-remote
defaults:
  ccache: true
`)
	err := os.WriteFile("aetherpak.yaml", data, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	// Ensure Viper is reset before loading
	viper.Reset()
	initConfig()

	// Run in rich mode (plain = false)
	logger.Init(false, false, false)
	buf := new(bytes.Buffer)
	configShowCmd.SetOut(buf)
	err = configShowCmd.RunE(configShowCmd, []string{})
	if err != nil {
		t.Fatalf("failed to run config show in rich mode: %v", err)
	}
	output := buf.String()

	if !strings.Contains(output, "AETHERPAK RESOLVED CONFIGURATION") {
		t.Errorf("expected output to contain rich header title, got %q", output)
	}
	if !strings.Contains(output, "registry:") {
		t.Errorf("expected output to contain 'registry:', got %q", output)
	}
}
