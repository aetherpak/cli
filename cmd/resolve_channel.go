package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/aetherpak/aetherpak/pkg/config"
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
	RunE: func(cmd *cobra.Command, args []string) error {
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

		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}

		channel, err := resolveChannel(cfg, refType, refName, defaultBranch)
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}
		if channel == "" {
			return NewCmdErrorf(1, "no git reference information available to resolve channel")
		}
		fmt.Println(channel)
		return nil
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
func resolveChannel(cfg *config.Config, refType, refName, defaultBranch string) (string, error) {
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

	if cfg != nil && len(cfg.ChannelMappings) > 0 {
		if refName != "" {
			if mapped, exists := cfg.ChannelMappings[refName]; exists {
				return mapped, nil
			}
			if target, matched := matchChannel(refName, cfg.ChannelMappings); matched {
				return target, nil
			}
		}
		if resolved != "" {
			if mapped, exists := cfg.ChannelMappings[resolved]; exists {
				return mapped, nil
			}
			if target, matched := matchChannel(resolved, cfg.ChannelMappings); matched {
				return target, nil
			}
		}
	}

	return resolved, nil
}

func matchChannel(val string, mappings map[string]string) (string, bool) {
	type match struct {
		pattern string
		target  string
	}
	var matches []match
	for pattern, target := range mappings {
		matched, err := filepath.Match(pattern, val)
		if err != nil {
			continue
		}
		if matched {
			matches = append(matches, match{pattern: pattern, target: target})
		}
	}
	if len(matches) > 0 {
		sort.Slice(matches, func(i, j int) bool {
			if len(matches[i].pattern) != len(matches[j].pattern) {
				return len(matches[i].pattern) > len(matches[j].pattern)
			}
			return matches[i].pattern < matches[j].pattern
		})
		return matches[0].target, true
	}
	return "", false
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
func resolveChannelFromEnv(cfg *config.Config) string {
	refType := getEnvRefType()
	refName := getEnvRefName()
	defaultBranch := getEnvDefaultBranch()
	if refType == "" && refName == "" {
		return ""
	}
	ch, _ := resolveChannel(cfg, refType, refName, defaultBranch)
	return ch
}
