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
	ci := os.Getenv("CI")
	if ci != "" {
		os.Unsetenv("CI")
		defer os.Setenv("CI", ci)
	}
	logger.Init(false, false, false)
	defer logger.Init(false, false, ci != "")

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

func TestConfigShowSecretsMasking(t *testing.T) {
	// Create empty config file
	err := os.WriteFile("aetherpak.yaml", []byte("{}"), 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	// Set GPG and OCI secrets in environment
	os.Setenv("AETHERPAK_GPG_KEY", "super-secret-gpg-key-payload")
	os.Setenv("AETHERPAK_GPG_KEY_PASSPHRASE", "my-gpg-passphrase")
	os.Setenv("OCI_PASSWORD", "my-oci-password")
	defer func() {
		os.Unsetenv("AETHERPAK_GPG_KEY")
		os.Unsetenv("AETHERPAK_GPG_KEY_PASSPHRASE")
		os.Unsetenv("OCI_PASSWORD")
	}()

	viper.Reset()
	initConfig()

	logger.Init(false, false, true) // force plain mode
	buf := new(bytes.Buffer)
	configShowCmd.SetOut(buf)

	// Create dummy command to test flag masking
	dummyCmd := configShowCmd
	dummyCmd.Flags().String("gpg-key-passphrase", "", "")
	dummyCmd.Flags().String("gpg-key", "", "")
	_ = dummyCmd.Flags().Set("gpg-key-passphrase", "flag-gpg-passphrase")
	_ = dummyCmd.Flags().Set("gpg-key", "flag-gpg-key")

	defer func() {
		// Clean up dynamically added flags so they don't leak to other tests
		// Reset flagset by creating a new one if necessary, or just ignore since we rebuild cmd
	}()

	err = configShowCmd.RunE(dummyCmd, []string{})
	if err != nil {
		t.Fatalf("failed to run config show: %v", err)
	}
	output := buf.String()

	// Assertions for unmasked values (should NOT be in output)
	if strings.Contains(output, "super-secret-gpg-key-payload") {
		t.Error("AETHERPAK_GPG_KEY value was leaked in cleartext!")
	}
	if strings.Contains(output, "my-gpg-passphrase") {
		t.Error("AETHERPAK_GPG_KEY_PASSPHRASE value was leaked in cleartext!")
	}
	if strings.Contains(output, "my-oci-password") {
		t.Error("OCI_PASSWORD value was leaked in cleartext!")
	}
	if strings.Contains(output, "flag-gpg-passphrase") {
		t.Error("gpg-key-passphrase flag value was leaked in cleartext!")
	}
	if strings.Contains(output, "flag-gpg-key") {
		t.Error("gpg-key flag value was leaked in cleartext!")
	}

	// Assertions for masked values (should be in output)
	if !strings.Contains(output, "AETHERPAK_GPG_KEY=********") {
		t.Error("expected AETHERPAK_GPG_KEY to be masked in environment overrides listing")
	}
	if !strings.Contains(output, "AETHERPAK_GPG_KEY_PASSPHRASE=********") {
		t.Error("expected AETHERPAK_GPG_KEY_PASSPHRASE to be masked in environment overrides listing")
	}
	if !strings.Contains(output, "OCI_PASSWORD=********") {
		t.Error("expected OCI_PASSWORD to be masked in environment overrides listing")
	}
	if !strings.Contains(output, "--gpg-key-passphrase=********") {
		t.Error("expected --gpg-key-passphrase flag override to be masked")
	}
	if !strings.Contains(output, "--gpg-key=********") {
		t.Error("expected --gpg-key flag override to be masked")
	}
}
