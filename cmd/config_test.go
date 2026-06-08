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
	// 1. Create a config with comments and specific ordering.
	err := os.WriteFile("aetherpak.yaml", []byte(`# top comment
registry: ghcr.io
remote_name: old-remote  # inline
apps:
  - id: org.first.App
    manifest: first.yaml
`), 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	// 2. Test set flat key
	err = configSetCmd.RunE(configSetCmd, []string{"remote_name", "my-custom-remote"})
	if err != nil {
		t.Fatalf("failed to set remote_name: %v", err)
	}

	// 3. Verify value was set and ordering/comments preserved.
	updatedData, err := os.ReadFile("aetherpak.yaml")
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}
	s := string(updatedData)
	if !strings.Contains(s, "remote_name: my-custom-remote") {
		t.Errorf("expected remote_name to be 'my-custom-remote', got:\n%s", s)
	}
	if !strings.Contains(s, "# top comment") {
		t.Errorf("top comment lost:\n%s", s)
	}
	if !strings.Contains(s, "# inline") {
		t.Errorf("inline comment lost:\n%s", s)
	}
	// registry must appear before remote_name
	if strings.Index(s, "registry:") >= strings.Index(s, "remote_name:") {
		t.Errorf("key ordering not preserved:\n%s", s)
	}
	// apps must appear after top-level keys
	if strings.Index(s, "remote_name:") >= strings.Index(s, "apps:") {
		t.Errorf("apps ordering not preserved:\n%s", s)
	}

	// 4. Test set nested key
	err = configSetCmd.RunE(configSetCmd, []string{"branding.logo_url", "https://logo.png"})
	if err != nil {
		t.Fatalf("failed to set branding.logo_url: %v", err)
	}
	updatedData, err = os.ReadFile("aetherpak.yaml")
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(updatedData, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	branding, ok := m["branding"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected branding to be map, got %T", m["branding"])
	}
	if branding["logo_url"] != "https://logo.png" {
		t.Errorf("expected branding.logo_url to be 'https://logo.png', got %v", branding["logo_url"])
	}

	// 5. Test set boolean key
	err = configSetCmd.RunE(configSetCmd, []string{"no_sign", "true"})
	if err != nil {
		t.Fatalf("failed to set no_sign: %v", err)
	}
	updatedData, err = os.ReadFile("aetherpak.yaml")
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}
	if err := yaml.Unmarshal(updatedData, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if m["no_sign"] != true {
		t.Errorf("expected no_sign to be true (boolean), got %v (type %T)", m["no_sign"], m["no_sign"])
	}

	// 6. Test validation: unknown key should fail
	err = configSetCmd.RunE(configSetCmd, []string{"foobar_invalid", "xyz"})
	if err == nil {
		t.Error("expected error for unknown key, got nil")
	} else if !strings.Contains(err.Error(), "unknown configuration key") {
		t.Errorf("unexpected error message: %v", err)
	}

	// 7. Test validation: invalid boolean should fail
	err = configSetCmd.RunE(configSetCmd, []string{"no_sign", "banana"})
	if err == nil {
		t.Error("expected error for invalid boolean value, got nil")
	} else if !strings.Contains(err.Error(), "invalid boolean value") {
		t.Errorf("unexpected error for invalid bool: %v", err)
	}

	// 8. Test list values (comma-separated)
	err = configSetCmd.RunE(configSetCmd, []string{"linter.ignore_rules", "rule1,rule2"})
	if err != nil {
		t.Fatalf("failed to set linter.ignore_rules: %v", err)
	}
	updatedData, err = os.ReadFile("aetherpak.yaml")
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if err := yaml.Unmarshal(updatedData, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	linter, ok := m["linter"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected linter to be map, got %T", m["linter"])
	}
	rules, ok := linter["ignore_rules"].([]interface{})
	if !ok {
		t.Fatalf("expected ignore_rules to be list, got %T", linter["ignore_rules"])
	}
	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d: %v", len(rules), rules)
	}

	// 9. Test list values (multiple args)
	err = configSetCmd.RunE(configSetCmd, []string{"defaults.builder_args", "--foo", "--bar"})
	if err != nil {
		t.Fatalf("failed to set defaults.builder_args: %v", err)
	}
	updatedData, err = os.ReadFile("aetherpak.yaml")
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if err := yaml.Unmarshal(updatedData, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	defaults, ok := m["defaults"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected defaults to be map, got %T", m["defaults"])
	}
	builderArgs, ok := defaults["builder_args"].([]interface{})
	if !ok {
		t.Fatalf("expected builder_args to be list, got %T", defaults["builder_args"])
	}
	if len(builderArgs) != 2 || builderArgs[0] != "--foo" || builderArgs[1] != "--bar" {
		t.Errorf("expected [--foo, --bar], got %v", builderArgs)
	}
}
