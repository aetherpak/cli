package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func TestConfigGet(t *testing.T) {
	// 1. Create mock config file
	data := []byte(`
remote_name: config-remote
branding:
  logo_url: https://example.com/logo.png
`)
	err := os.WriteFile("aetherpak.yaml", data, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	// Ensure Viper is reset before loading
	viper.Reset()
	initConfig()
	logger.Init(false, false, true) // ensure plain mode for tests

	// 2. Test get flat key
	buf := new(bytes.Buffer)
	configGetCmd.SetOut(buf)
	err = configGetCmd.RunE(configGetCmd, []string{"remote_name"})
	if err != nil {
		t.Fatalf("failed to run config get: %v", err)
	}
	output := strings.TrimSpace(buf.String())
	if output != "config-remote" {
		t.Errorf("expected 'config-remote', got %q", output)
	}

	// 3. Test get nested key
	buf.Reset()
	err = configGetCmd.RunE(configGetCmd, []string{"branding.logo_url"})
	if err != nil {
		t.Fatalf("failed to run config get nested: %v", err)
	}
	output = strings.TrimSpace(buf.String())
	if output != "https://example.com/logo.png" {
		t.Errorf("expected 'https://example.com/logo.png', got %q", output)
	}

	// Test get complex key (map formatted as YAML)
	buf.Reset()
	err = configGetCmd.RunE(configGetCmd, []string{"branding"})
	if err != nil {
		t.Fatalf("failed to run config get complex: %v", err)
	}
	output = strings.TrimSpace(buf.String())
	if !strings.Contains(output, "logo_url: https://example.com/logo.png") {
		t.Errorf("expected output to contain 'logo_url: https://example.com/logo.png', got %q", output)
	}

	// 4. Test environment variable override
	os.Setenv("AETHERPAK_REMOTE_NAME", "env-remote")
	defer os.Unsetenv("AETHERPAK_REMOTE_NAME")

	viper.Reset()
	initConfig()

	buf.Reset()
	err = configGetCmd.RunE(configGetCmd, []string{"remote_name"})
	if err != nil {
		t.Fatalf("failed to run config get with env var: %v", err)
	}
	output = strings.TrimSpace(buf.String())
	// 5. Test get with rich styling (plain = false)
	ci := os.Getenv("CI")
	if ci != "" {
		os.Unsetenv("CI")
		defer os.Setenv("CI", ci)
	}
	logger.Init(false, false, false)
	defer logger.Init(false, false, ci != "")
	lipgloss.SetColorProfile(termenv.ANSI)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	buf.Reset()
	err = configGetCmd.RunE(configGetCmd, []string{"remote_name"})
	if err != nil {
		t.Fatalf("failed to run config get in rich mode: %v", err)
	}
	output = buf.String()
	if !strings.Contains(output, "\x1b") {
		t.Errorf("expected ANSI escape characters in rich output, got %q", output)
	}
	if !strings.Contains(output, "env-remote") {
		t.Errorf("expected output to contain 'env-remote', got %q", output)
	}
}

func TestConfigSet(t *testing.T) {
	// 1. Create empty config file
	err := os.WriteFile("aetherpak.yaml", []byte(`{}`), 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	// 2. Test set flat key
	err = configSetCmd.RunE(configSetCmd, []string{"remote_name", "my-custom-remote"})
	if err != nil {
		t.Fatalf("failed to set remote_name: %v", err)
	}

	// 3. Test set nested key
	err = configSetCmd.RunE(configSetCmd, []string{"branding.logo_url", "https://logo.png"})
	if err != nil {
		t.Fatalf("failed to set branding.logo_url: %v", err)
	}

	// 4. Test set boolean key
	err = configSetCmd.RunE(configSetCmd, []string{"no_sign", "true"})
	if err != nil {
		t.Fatalf("failed to set no_sign: %v", err)
	}

	// 5. Test set integer key
	err = configSetCmd.RunE(configSetCmd, []string{"defaults.ccache_dir", "1234"})
	if err != nil {
		t.Fatalf("failed to set defaults.ccache_dir: %v", err)
	}

	// 6. Verify contents written to file
	updatedData, err := os.ReadFile("aetherpak.yaml")
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}

	var m map[string]interface{}
	if err := yaml.Unmarshal(updatedData, &m); err != nil {
		t.Fatalf("failed to unmarshal updated config: %v", err)
	}

	if m["remote_name"] != "my-custom-remote" {
		t.Errorf("expected remote_name to be 'my-custom-remote', got %v", m["remote_name"])
	}

	branding, ok := m["branding"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected branding to be map, got %T", m["branding"])
	}
	if branding["logo_url"] != "https://logo.png" {
		t.Errorf("expected branding.logo_url to be 'https://logo.png', got %v", branding["logo_url"])
	}

	if m["no_sign"] != true {
		t.Errorf("expected no_sign to be true (boolean), got %v (type %T)", m["no_sign"], m["no_sign"])
	}

	defaults, ok := m["defaults"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected defaults to be map, got %T", m["defaults"])
	}
	if defaults["ccache_dir"] != 1234 {
		t.Errorf("expected defaults.ccache_dir to be 1234 (int), got %v (type %T)", defaults["ccache_dir"], defaults["ccache_dir"])
	}
}

func TestConfigSetPreservesCommentsAndValidates(t *testing.T) {
	// 1. Setup config with comments
	existing := []byte(`# Top level comment
registry: old.registry.io # inline registry
# Branding comment
branding:
  logo_url: https://old.logo.png # logo inline
`)
	err := os.WriteFile("aetherpak.yaml", existing, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	viper.Reset()
	defer viper.Reset()
	initConfig()

	// 2. Set registry (should preserve comments)
	err = configSetCmd.RunE(configSetCmd, []string{"registry", "new.registry.io"})
	if err != nil {
		t.Fatalf("failed to set registry: %v", err)
	}

	contentBytes, err := os.ReadFile("aetherpak.yaml")
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	content := string(contentBytes)

	if !strings.Contains(content, "# Top level comment") {
		t.Error("expected top level comment to be preserved")
	}
	if !strings.Contains(content, "# inline registry") {
		t.Error("expected inline registry comment to be preserved")
	}
	if !strings.Contains(content, "registry: new.registry.io") {
		t.Errorf("expected registry to be updated, got:\n%s", content)
	}

	// 3. Try setting an invalid key (should fail validation and rollback)
	err = configSetCmd.RunE(configSetCmd, []string{"typo_key_name", "value"})
	if err == nil {
		t.Error("expected error setting invalid/typo key, got nil")
	}

	// Verify rollback occurred (invalid key not present in config file)
	contentBytes2, err := os.ReadFile("aetherpak.yaml")
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	content2 := string(contentBytes2)
	if strings.Contains(content2, "typo_key_name") {
		t.Error("expected invalid/typo key to be rolled back and not present in file")
	}
}
