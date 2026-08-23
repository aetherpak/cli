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

func TestConfigGetMultiApp(t *testing.T) {
	yamlData := []byte(`
registry: ghcr.io
remote_name: myorg/myrepo
apps:
  - id: org.example.App
    manifest: apps/org.example.App.json
    branch: stable
    runtime: org.gnome.Platform
    runtime-version: "45"
    run-linter: true
  - id: org.example.SourcesApp
    branch: main
    runtime: org.gnome.Platform//45
    sources:
      desktop: data/app.desktop
      binaries:
        - path: build/app
          dest: /app/bin/app
  - id: org.example.BundleApp
    branch: stable
    bundles:
      x86_64:
        url: https://example.com/app-x86_64.flatpak
        sha256: 1111111111111111111111111111111111111111111111111111111111111111
  - id: org.example.Other
    manifest: apps/org.example.Other.json
    branch: beta
    runtime: org.freedesktop.Platform
    runtime_version: "23.08"
`)
	err := os.WriteFile("aetherpak.yaml", yamlData, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	resetCmd := func() {
		viper.Reset()
		initConfig()
		logger.Init(false, false, true)
		configGetAppID = ""
		_ = configGetCmd.Flags().Set("app-id", "")
		configGetCmd.Flags().Lookup("app-id").Changed = false
	}

	tests := []struct {
		name     string
		appID    string
		args     []string
		expected string
	}{
		{
			name:     "path lookup manifest app 1",
			args:     []string{"apps.org.example.App.manifest"},
			expected: "apps/org.example.App.json",
		},
		{
			name:     "path lookup manifest app 2",
			args:     []string{"apps.org.example.Other.manifest"},
			expected: "apps/org.example.Other.json",
		},
		{
			name:     "path lookup branch app 1",
			args:     []string{"apps.org.example.App.branch"},
			expected: "stable",
		},
		{
			name:     "path lookup branch app 2",
			args:     []string{"apps.org.example.Other.branch"},
			expected: "beta",
		},
		{
			name:     "path lookup runtime",
			args:     []string{"apps.org.example.App.runtime"},
			expected: "org.gnome.Platform",
		},
		{
			name:     "path lookup runtime-version (kebab)",
			args:     []string{"apps.org.example.App.runtime-version"},
			expected: "45",
		},
		{
			name:     "path lookup runtime_version (snake)",
			args:     []string{"apps.org.example.App.runtime_version"},
			expected: "45",
		},
		{
			name:     "path lookup run-linter",
			args:     []string{"apps.org.example.App.run-linter"},
			expected: "true",
		},
		{
			name:     "path lookup run_linter",
			args:     []string{"apps.org.example.App.run_linter"},
			expected: "true",
		},
		{
			name:     "path lookup nested sources.desktop",
			args:     []string{"apps.org.example.SourcesApp.sources.desktop"},
			expected: "data/app.desktop",
		},
		{
			name:     "path lookup nested bundles url",
			args:     []string{"apps.org.example.BundleApp.bundles.x86_64.url"},
			expected: "https://example.com/app-x86_64.flatpak",
		},
		{
			name:     "flag lookup manifest app 1",
			appID:    "org.example.App",
			args:     []string{"manifest"},
			expected: "apps/org.example.App.json",
		},
		{
			name:     "flag lookup manifest app 2",
			appID:    "org.example.Other",
			args:     []string{"manifest"},
			expected: "apps/org.example.Other.json",
		},
		{
			name:     "flag lookup branch app 1",
			appID:    "org.example.App",
			args:     []string{"branch"},
			expected: "stable",
		},
		{
			name:     "flag lookup sources.desktop",
			appID:    "org.example.SourcesApp",
			args:     []string{"sources.desktop"},
			expected: "data/app.desktop",
		},
		{
			name:     "numeric index fallback apps.0.manifest",
			args:     []string{"apps.0.manifest"},
			expected: "apps/org.example.App.json",
		},
		{
			name:     "numeric index fallback apps.3.manifest",
			args:     []string{"apps.3.manifest"},
			expected: "apps/org.example.Other.json",
		},
		{
			name:     "nonexistent app ID with flag",
			appID:    "org.example.Nonexistent",
			args:     []string{"manifest"},
			expected: "",
		},
		{
			name:     "nonexistent app ID in path",
			args:     []string{"apps.org.example.Nonexistent.manifest"},
			expected: "",
		},
		{
			name:     "nonexistent field on valid app",
			appID:    "org.example.App",
			args:     []string{"nonexistent_field"},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetCmd()
			if tc.appID != "" {
				_ = configGetCmd.Flags().Set("app-id", tc.appID)
			}
			buf := new(bytes.Buffer)
			configGetCmd.SetOut(buf)
			err := configGetCmd.RunE(configGetCmd, tc.args)
			if err != nil {
				t.Fatalf("unexpected error running config get: %v", err)
			}
			got := strings.TrimSpace(buf.String())
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}

	// Test full app object retrieval via path: apps.org.example.App
	t.Run("full app retrieval via path", func(t *testing.T) {
		resetCmd()
		buf := new(bytes.Buffer)
		configGetCmd.SetOut(buf)
		err := configGetCmd.RunE(configGetCmd, []string{"apps.org.example.App"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := buf.String()
		if !strings.Contains(got, "org.example.App") || !strings.Contains(got, "apps/org.example.App.json") {
			t.Errorf("expected full app YAML, got: %s", got)
		}
	})

	// Test full app object retrieval via flag without args
	t.Run("full app retrieval via flag without args", func(t *testing.T) {
		resetCmd()
		_ = configGetCmd.Flags().Set("app-id", "org.example.App")
		buf := new(bytes.Buffer)
		configGetCmd.SetOut(buf)
		err := configGetCmd.RunE(configGetCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := buf.String()
		if !strings.Contains(got, "org.example.App") || !strings.Contains(got, "apps/org.example.App.json") {
			t.Errorf("expected full app YAML, got: %s", got)
		}
	})
}

func TestConfigGetHierarchicalAppIDs(t *testing.T) {
	yamlData := []byte(`
apps:
  - id: org.example.App
    manifest: apps/app.json
  - id: org.example.App.Plugin
    manifest: apps/plugin.json
`)
	err := os.WriteFile("aetherpak.yaml", yamlData, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	resetCmd := func() {
		viper.Reset()
		initConfig()
		logger.Init(false, false, true)
		configGetAppID = ""
		_ = configGetCmd.Flags().Set("app-id", "")
		configGetCmd.Flags().Lookup("app-id").Changed = false
	}

	t.Run("longer app ID matched correctly", func(t *testing.T) {
		resetCmd()
		buf := new(bytes.Buffer)
		configGetCmd.SetOut(buf)
		err := configGetCmd.RunE(configGetCmd, []string{"apps.org.example.App.Plugin.manifest"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := strings.TrimSpace(buf.String())
		if got != "apps/plugin.json" {
			t.Errorf("expected 'apps/plugin.json', got %q", got)
		}
	})

	t.Run("shorter app ID matched correctly", func(t *testing.T) {
		resetCmd()
		buf := new(bytes.Buffer)
		configGetCmd.SetOut(buf)
		err := configGetCmd.RunE(configGetCmd, []string{"apps.org.example.App.manifest"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := strings.TrimSpace(buf.String())
		if got != "apps/app.json" {
			t.Errorf("expected 'apps/app.json', got %q", got)
		}
	})
}

func TestConfigGetSingleAppZeroManifest(t *testing.T) {
	yamlData := []byte(`
app_id: org.example.Single
runtime: org.gnome.Platform//49
sources:
  desktop: data/single.desktop
`)
	err := os.WriteFile("aetherpak.yaml", yamlData, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	resetCmd := func() {
		viper.Reset()
		initConfig()
		logger.Init(false, false, true)
		configGetAppID = ""
		_ = configGetCmd.Flags().Set("app-id", "")
		configGetCmd.Flags().Lookup("app-id").Changed = false
	}

	t.Run("query by app-id flag", func(t *testing.T) {
		resetCmd()
		_ = configGetCmd.Flags().Set("app-id", "org.example.Single")
		buf := new(bytes.Buffer)
		configGetCmd.SetOut(buf)
		err := configGetCmd.RunE(configGetCmd, []string{"runtime"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := strings.TrimSpace(buf.String())
		if got != "org.gnome.Platform" {
			t.Errorf("expected 'org.gnome.Platform', got %q", got)
		}
	})

	t.Run("query by apps path", func(t *testing.T) {
		resetCmd()
		buf := new(bytes.Buffer)
		configGetCmd.SetOut(buf)
		err := configGetCmd.RunE(configGetCmd, []string{"apps.org.example.Single.sources.desktop"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := strings.TrimSpace(buf.String())
		if got != "data/single.desktop" {
			t.Errorf("expected 'data/single.desktop', got %q", got)
		}
	})
}

func TestConfigGetAppIDRefBranch(t *testing.T) {
	yamlData := []byte(`
apps:
  - id: org.example.App
    manifest: apps/org.example.App.json
    branch: stable
`)
	err := os.WriteFile("aetherpak.yaml", yamlData, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	resetCmd := func() {
		viper.Reset()
		initConfig()
		logger.Init(false, false, true)
		configGetAppID = ""
		_ = configGetCmd.Flags().Set("app-id", "")
		configGetCmd.Flags().Lookup("app-id").Changed = false
	}

	t.Run("app-id ref with branch via flag", func(t *testing.T) {
		resetCmd()
		_ = configGetCmd.Flags().Set("app-id", "org.example.App//beta")
		buf := new(bytes.Buffer)
		configGetCmd.SetOut(buf)
		err := configGetCmd.RunE(configGetCmd, []string{"manifest"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := strings.TrimSpace(buf.String())
		if got != "apps/org.example.App.json" {
			t.Errorf("expected 'apps/org.example.App.json', got %q", got)
		}
	})

	t.Run("app-id ref with branch via path", func(t *testing.T) {
		resetCmd()
		buf := new(bytes.Buffer)
		configGetCmd.SetOut(buf)
		err := configGetCmd.RunE(configGetCmd, []string{"apps.org.example.App//beta.manifest"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := strings.TrimSpace(buf.String())
		if got != "apps/org.example.App.json" {
			t.Errorf("expected 'apps/org.example.App.json', got %q", got)
		}
	})
}
