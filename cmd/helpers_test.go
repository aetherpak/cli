package cmd

import (
	"os"
	"reflect"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/spf13/cobra"
)

func TestResolveLinterExceptions(t *testing.T) {
	// Backup ambient env
	envVal1 := os.Getenv("AETHERPAK_LINTER_EXCEPTIONS_FILE")
	envVal2 := os.Getenv("AETHERPAK_LINTER_EXCEPTIONS")
	defer func() {
		os.Setenv("AETHERPAK_LINTER_EXCEPTIONS_FILE", envVal1)
		os.Setenv("AETHERPAK_LINTER_EXCEPTIONS", envVal2)
	}()

	os.Unsetenv("AETHERPAK_LINTER_EXCEPTIONS_FILE")
	os.Unsetenv("AETHERPAK_LINTER_EXCEPTIONS")

	// 1. Basic defaults
	rules, file := resolveLinterExceptions(
		false, false,
		[]string{"rule1"}, "default.json",
		nil, "",
	)
	if file != "default.json" || len(rules) != 1 || rules[0] != "rule1" {
		t.Errorf("expected defaults, got rules=%v file=%q", rules, file)
	}

	// 2. Env file override
	os.Setenv("AETHERPAK_LINTER_EXCEPTIONS_FILE", "env-file.json")
	_, file = resolveLinterExceptions(false, false, nil, "default.json", nil, "")
	if file != "env-file.json" {
		t.Errorf("expected env-file.json, got %q", file)
	}
	os.Unsetenv("AETHERPAK_LINTER_EXCEPTIONS_FILE")

	// 3. Env list of rules override
	os.Setenv("AETHERPAK_LINTER_EXCEPTIONS", "ruleA, ruleB")
	rules, _ = resolveLinterExceptions(false, false, []string{"defaultRule"}, "", nil, "")
	expectedRules := []string{"ruleA", "ruleB"}
	if !reflect.DeepEqual(rules, expectedRules) {
		t.Errorf("expected rules %v, got %v", expectedRules, rules)
	}

	// 4. Env JSON file override inside GITHUB_LINTER_EXCEPTIONS alias
	os.Setenv("AETHERPAK_LINTER_EXCEPTIONS", "env-rule-file.json")
	_, file = resolveLinterExceptions(false, false, nil, "default.json", nil, "")
	if file != "env-rule-file.json" {
		t.Errorf("expected env-rule-file.json, got %q", file)
	}
	os.Unsetenv("AETHERPAK_LINTER_EXCEPTIONS")

	// 5. Flag overrides take precedence
	rules, file = resolveLinterExceptions(
		true, true,
		[]string{"default"}, "default.json",
		[]string{"flag"}, "flag.json",
	)
	if file != "flag.json" || len(rules) != 1 || rules[0] != "flag" {
		t.Errorf("expected flag override, got rules=%v file=%q", rules, file)
	}
}

func TestSanitizeRemoteName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Flathub", "flathub"},
		{"My Custom Repo!", "my-custom-repo-"},
		{"app-ref_name", "app-ref_name"},
	}

	for _, tt := range tests {
		actual := sanitizeRemoteName(tt.input)
		if actual != tt.expected {
			t.Errorf("sanitizeRemoteName(%q) = %q; expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestSplitAndCleanSlice(t *testing.T) {
	tests := []struct {
		input    []string
		expected []string
	}{
		{
			input:    []string{"a, b, c"},
			expected: []string{"a", "b", "c"},
		},
		{
			input:    []string{"a\nb\nc"},
			expected: []string{"a", "b", "c"},
		},
		{
			input:    []string{"a,b\n   c  , d\r\n e"},
			expected: []string{"a", "b", "c", "d", "e"},
		},
		{
			input:    []string{"", "  ", "a", "b"},
			expected: []string{"a", "b"},
		},
		{
			input:    []string{"a, b", "c\nd, e"},
			expected: []string{"a", "b", "c", "d", "e"},
		},
	}

	for _, tt := range tests {
		actual := SplitAndCleanSlice(tt.input)
		if !reflect.DeepEqual(actual, tt.expected) {
			t.Errorf("SplitAndCleanSlice(%v) = %v; expected %v", tt.input, actual, tt.expected)
		}
	}
}

func TestParseAppIDRef(t *testing.T) {
	tests := []struct {
		input      string
		expectedID string
		expectedBr string
	}{
		{"org.gnome.Sudoku", "org.gnome.Sudoku", ""},
		{"org.gnome.Sudoku//beta", "org.gnome.Sudoku", "beta"},
		{"org.gnome.Sudoku//", "org.gnome.Sudoku", ""},
		{"//beta", "", "beta"},
		{"", "", ""},
	}

	for _, tt := range tests {
		id, br := parseAppIDRef(tt.input)
		if id != tt.expectedID || br != tt.expectedBr {
			t.Errorf("parseAppIDRef(%q) = (%q, %q); expected (%q, %q)", tt.input, id, br, tt.expectedID, tt.expectedBr)
		}
	}
}

func TestResolveBuildOptions(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("ccache-dir", "", "")
	cmd.Flags().String("state-dir", "", "")
	cmd.Flags().Bool("run-linter", false, "")
	cmd.Flags().Bool("no-sign", false, "")
	cmd.Flags().Bool("no-install-deps", false, "")
	cmd.Flags().Bool("no-flathub", false, "")
	cmd.Flags().StringSlice("builder-arg", nil, "")
	cmd.Flags().StringArray("flatpak-remote", nil, "")
	cmd.Flags().StringArray("flatpak-dep", nil, "")
	cmd.Flags().String("linter-exceptions-file", "", "")
	cmd.Flags().StringSlice("linter-exception", nil, "")
	cmd.Flags().Bool("install", false, "")
	cmd.Flags().Bool("bundle", false, "")

	cfg := &config.Config{
		NoSign: false,
		Defaults: &config.DefaultsConfig{
			CCacheDir:   ".ccache-cfg",
			StateDir:    ".state-cfg",
			RunLinter:   true,
			BuilderArgs: []string{"--foo"},
		},
	}

	appCfg := &config.App{
		ID:          "org.example.App",
		CCacheDir:   ".ccache-app",
		StateDir:    ".state-app",
		RunLinter:   false,
		BuilderArgs: []string{"--bar"},
	}

	// 1. App configuration precedence
	opts, err := resolveBuildOptions(cmd, cfg, appCfg, "org.example.App", "manifest.json", "x86_64", "stable", "repo")
	if err != nil {
		t.Fatalf("resolveBuildOptions failed: %v", err)
	}
	if opts.CCacheDir != ".ccache-app" || opts.StateDir != ".state-app" || opts.RunLinter != false || !reflect.DeepEqual(opts.BuilderArgs, []string{"--bar"}) {
		t.Errorf("unexpected build options from app configuration: %+v", opts)
	}

	// 2. Global defaults fallback
	opts, err = resolveBuildOptions(cmd, cfg, nil, "org.example.App", "manifest.json", "x86_64", "stable", "repo")
	if err != nil {
		t.Fatalf("resolveBuildOptions failed: %v", err)
	}
	if opts.CCacheDir != ".ccache-cfg" || opts.StateDir != ".state-cfg" || opts.RunLinter != true || !reflect.DeepEqual(opts.BuilderArgs, []string{"--foo"}) {
		t.Errorf("unexpected build options from global defaults: %+v", opts)
	}

	// 3. CLI flag overrides
	_ = cmd.Flags().Set("ccache-dir", ".ccache-override")
	_ = cmd.Flags().Set("state-dir", ".state-override")
	_ = cmd.Flags().Set("run-linter", "false")
	_ = cmd.Flags().Set("no-sign", "true")
	_ = cmd.Flags().Set("install", "true")
	_ = cmd.Flags().Set("bundle", "true")

	opts, err = resolveBuildOptions(cmd, cfg, nil, "org.example.App", "manifest.json", "x86_64", "stable", "repo")
	if err != nil {
		t.Fatalf("resolveBuildOptions failed: %v", err)
	}
	if opts.CCacheDir != ".ccache-override" || opts.StateDir != ".state-override" || opts.RunLinter != false || opts.NoSign != true || opts.Install != true || opts.Bundle != true {
		t.Errorf("unexpected build options with CLI flag overrides: %+v", opts)
	}
}
