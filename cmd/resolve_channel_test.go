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
			actual := resolveChannel(tt.refType, tt.refName, tt.defaultBranch)
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
			actual := resolveChannel(tt.refType, tt.refName, tt.defaultBranch)
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}
