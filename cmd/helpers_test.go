package cmd

import (
	"os"
	"reflect"
	"testing"
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
