package cmd

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

func TestResolveChannel(t *testing.T) {
	tests := []struct {
		name          string
		refType       string
		refName       string
		defaultBranch string
		expected      string
	}{
		{
			name:          "tag ref resolves to stable",
			refType:       "tag",
			refName:       "v1.0.0",
			defaultBranch: "main",
			expected:      "stable",
		},
		{
			name:          "default branch ref name resolves to beta",
			refType:       "branch",
			refName:       "main",
			defaultBranch: "main",
			expected:      "beta",
		},
		{
			name:          "default branch empty fallback main resolves to beta",
			refType:       "branch",
			refName:       "main",
			defaultBranch: "",
			expected:      "beta",
		},
		{
			name:          "other branch resolves to branch name",
			refType:       "branch",
			refName:       "feature-123",
			defaultBranch: "main",
			expected:      "feature-123",
		},
		{
			name:          "empty info returns empty",
			refType:       "",
			refName:       "",
			defaultBranch: "",
			expected:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear viper state to make resolveChannel clean of any testing files
			viper.Reset()
			actual, err := resolveChannel(tt.refType, tt.refName, tt.defaultBranch)
			if err != nil {
				t.Fatalf("unexpected error resolving channel: %v", err)
			}
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestResolveChannelWithMappings(t *testing.T) {
	configData := []byte(`
channel_mappings:
  "main": "beta"
  "staging/*": "alpha"
  "release-*": "stable"
  "stable": "prod"
`)
	err := os.WriteFile("aetherpak.yaml", configData, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	// Reset viper config state to force re-reading
	viper.Reset()
	initConfig()

	tests := []struct {
		name          string
		refType       string
		refName       string
		defaultBranch string
		expected      string
	}{
		{
			name:          "exact match on refName main",
			refType:       "branch",
			refName:       "main",
			defaultBranch: "main",
			expected:      "beta",
		},
		{
			name:          "wildcard match staging/*",
			refType:       "branch",
			refName:       "staging/patch-1",
			defaultBranch: "main",
			expected:      "alpha",
		},
		{
			name:          "wildcard match release-*",
			refType:       "branch",
			refName:       "release-1.0.0",
			defaultBranch: "main",
			expected:      "stable",
		},
		{
			name:          "exact match on resolved channel stable",
			refType:       "tag",
			refName:       "v1.0.0",
			defaultBranch: "main",
			expected:      "prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := resolveChannel(tt.refType, tt.refName, tt.defaultBranch)
			if err != nil {
				t.Fatalf("unexpected error resolving channel: %v", err)
			}
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestResolveChannelFromEnv(t *testing.T) {
	// Clear viper state to prevent config bleed from other tests
	viper.Reset()

	// Backup ambient env
	envVars := []string{
		"AETHERPAK_REF_TYPE", "AETHERPAK_REF_NAME", "AETHERPAK_DEFAULT_BRANCH",
		"GITHUB_REF_TYPE", "GITHUB_REF_NAME",
		"CI_COMMIT_TAG", "CI_COMMIT_BRANCH", "CI_COMMIT_REF_NAME", "CI_DEFAULT_BRANCH",
		"DEFAULT_BRANCH",
	}
	backup := make(map[string]string)
	for _, k := range envVars {
		backup[k] = os.Getenv(k)
	}
	defer func() {
		for _, k := range envVars {
			if v := backup[k]; v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	// Clear all env vars for test environment isolation
	for _, k := range envVars {
		os.Unsetenv(k)
	}

	// 1. Unset env returns empty
	if ch := resolveChannelFromEnv(); ch != "" {
		t.Errorf("expected empty resolution, got %q", ch)
	}

	// 2. AETHERPAK_* variables take precedence
	os.Setenv("AETHERPAK_REF_TYPE", "tag")
	os.Setenv("AETHERPAK_REF_NAME", "v1.2.3")
	if ch := resolveChannelFromEnv(); ch != "stable" {
		t.Errorf("expected stable from AETHERPAK_ env, got %q", ch)
	}

	os.Setenv("AETHERPAK_REF_TYPE", "branch")
	os.Setenv("AETHERPAK_REF_NAME", "mybranch")
	os.Setenv("AETHERPAK_DEFAULT_BRANCH", "mybranch")
	if ch := resolveChannelFromEnv(); ch != "beta" {
		t.Errorf("expected beta from AETHERPAK_ env matching default branch, got %q", ch)
	}

	// 3. GITHUB_* variables fallback
	os.Unsetenv("AETHERPAK_REF_TYPE")
	os.Unsetenv("AETHERPAK_REF_NAME")
	os.Unsetenv("AETHERPAK_DEFAULT_BRANCH")

	os.Setenv("GITHUB_REF_TYPE", "tag")
	os.Setenv("GITHUB_REF_NAME", "v2.0.0")
	if ch := resolveChannelFromEnv(); ch != "stable" {
		t.Errorf("expected stable from GITHUB_ env, got %q", ch)
	}

	// 4. GitLab / general CI fallback
	os.Unsetenv("GITHUB_REF_TYPE")
	os.Unsetenv("GITHUB_REF_NAME")

	os.Setenv("CI_COMMIT_TAG", "v3.0.0")
	if ch := resolveChannelFromEnv(); ch != "stable" {
		t.Errorf("expected stable from CI_COMMIT_TAG, got %q", ch)
	}

	os.Unsetenv("CI_COMMIT_TAG")
	os.Setenv("CI_COMMIT_BRANCH", "dev")
	os.Setenv("CI_DEFAULT_BRANCH", "main")
	if ch := resolveChannelFromEnv(); ch != "dev" {
		t.Errorf("expected dev branch resolved channel, got %q", ch)
	}
}
