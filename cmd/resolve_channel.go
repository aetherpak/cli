package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	rcRefType       string
	rcRefName       string
	rcDefaultBranch string
)

var resolveChannelCmd = &cobra.Command{
	Use:   "resolve-channel",
	Short: "Resolves the flatpak channel name from git ref metadata",
	Long: `Determines the appropriate flatpak channel (stable, beta, or ref name)
based on CI environment variables (supports GitHub Actions, GitLab CI, and AetherPak env overrides).

Rules:
  tag           → stable
  default branch → beta
  other          → the ref name itself`,
	Run: func(cmd *cobra.Command, args []string) {
		refType := rcRefType
		if refType == "" {
			refType = getEnvRefType()
		}
		refName := rcRefName
		if refName == "" {
			refName = getEnvRefName()
		}
		defaultBranch := rcDefaultBranch
		if defaultBranch == "" {
			defaultBranch = getEnvDefaultBranch()
		}

		channel := resolveChannel(refType, refName, defaultBranch)
		fmt.Println(channel)
	},
}

func init() {
	RootCmd.AddCommand(resolveChannelCmd)

	resolveChannelCmd.Flags().StringVar(&rcRefType, "ref-type", "", "git ref type (e.g. tag, branch); defaults to CI environment variables")
	resolveChannelCmd.Flags().StringVar(&rcRefName, "ref-name", "", "git ref name; defaults to CI environment variables")
	resolveChannelCmd.Flags().StringVar(&rcDefaultBranch, "default-branch", "", "default branch name; defaults to CI environment variables or 'main'")
}

// resolveChannel applies the channel resolution rules:
//
//	tag            → stable
//	default branch → beta
//	other          → ref name
func resolveChannel(refType, refName, defaultBranch string) string {
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	var resolved string
	if refType == "tag" {
		resolved = "stable"
	} else if refName == defaultBranch {
		resolved = "beta"
	} else if refName != "" {
		resolved = refName
	}

	if cfg, err := LoadConfig(); err == nil && cfg != nil && len(cfg.ChannelMappings) > 0 {
		if refName != "" {
			if mapped, exists := cfg.ChannelMappings[refName]; exists {
				return mapped
			}
			for pattern, target := range cfg.ChannelMappings {
				if matched, _ := filepath.Match(pattern, refName); matched {
					return target
				}
			}
		}
		if resolved != "" {
			if mapped, exists := cfg.ChannelMappings[resolved]; exists {
				return mapped
			}
			for pattern, target := range cfg.ChannelMappings {
				if matched, _ := filepath.Match(pattern, resolved); matched {
					return target
				}
			}
		}
	}

	return resolved
}

func getEnvRefType() string {
	if val := os.Getenv("AETHERPAK_REF_TYPE"); val != "" {
		return val
	}
	if val := os.Getenv("GITHUB_REF_TYPE"); val != "" {
		return val
	}
	if os.Getenv("CI_COMMIT_TAG") != "" {
		return "tag"
	}
	if os.Getenv("CI_COMMIT_BRANCH") != "" {
		return "branch"
	}
	return ""
}

func getEnvRefName() string {
	if val := os.Getenv("AETHERPAK_REF_NAME"); val != "" {
		return val
	}
	if val := os.Getenv("GITHUB_REF_NAME"); val != "" {
		return val
	}
	if val := os.Getenv("CI_COMMIT_TAG"); val != "" {
		return val
	}
	if val := os.Getenv("CI_COMMIT_BRANCH"); val != "" {
		return val
	}
	if val := os.Getenv("CI_COMMIT_REF_NAME"); val != "" {
		return val
	}
	return ""
}

func getEnvDefaultBranch() string {
	if val := os.Getenv("AETHERPAK_DEFAULT_BRANCH"); val != "" {
		return val
	}
	if val := os.Getenv("DEFAULT_BRANCH"); val != "" {
		return val
	}
	if val := os.Getenv("CI_DEFAULT_BRANCH"); val != "" {
		return val
	}
	return "main"
}

// resolveChannelFromEnv returns the channel name derived from environment
// variables, or empty string if the environment doesn't provide enough
// information (e.g. running outside CI).
func resolveChannelFromEnv() string {
	refType := getEnvRefType()
	refName := getEnvRefName()
	defaultBranch := getEnvDefaultBranch()
	if refType == "" && refName == "" {
		return ""
	}
	return resolveChannel(refType, refName, defaultBranch)
}
