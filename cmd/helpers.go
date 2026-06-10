package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/builder"
	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/spf13/cobra"
)

// resolveLinterExceptions retrieves the final set of linter exceptions and exceptions file,
// merging configuration defaults/overrides and applying environment variables and CLI flag overrides.
func resolveLinterExceptions(
	linterExceptionsFileChanged bool,
	linterExceptionChanged bool,
	defaultExceptions []string,
	defaultExceptionsFile string,
	flagExceptions []string,
	flagExceptionsFile string,
) ([]string, string) {
	exceptions := defaultExceptions
	exceptionsFile := defaultExceptionsFile

	if envVal := os.Getenv("AETHERPAK_LINTER_EXCEPTIONS_FILE"); envVal != "" {
		exceptionsFile = envVal
	} else if envVal := os.Getenv("AETHERPAK_LINTER_EXCEPTIONS"); envVal != "" {
		if strings.HasSuffix(envVal, ".json") {
			exceptionsFile = envVal
		} else {
			var envList []string
			for _, item := range strings.Split(envVal, ",") {
				item = strings.TrimSpace(item)
				if item != "" {
					envList = append(envList, item)
				}
			}
			if len(envList) > 0 {
				exceptions = envList
			}
		}
	}

	if linterExceptionsFileChanged {
		exceptionsFile = flagExceptionsFile
	}
	if linterExceptionChanged {
		exceptions = flagExceptions
	}

	return exceptions, exceptionsFile
}

func parseFlatpakRemotes(remotes []string) (map[string]config.RemoteConfig, error) {
	out := make(map[string]config.RemoteConfig)
	for _, r := range remotes {
		parts := strings.SplitN(r, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid flatpak remote format %q, must be name=url", r)
		}
		out[parts[0]] = config.RemoteConfig{URL: parts[1]}
	}
	return out, nil
}

func parseFlatpakDeps(deps []string) ([]config.FlatpakDep, error) {
	var out []config.FlatpakDep
	for _, d := range deps {
		var parts []string
		if strings.Contains(d, ":") {
			parts = strings.SplitN(d, ":", 2)
		} else if strings.Contains(d, "=") {
			parts = strings.SplitN(d, "=", 2)
		} else {
			return nil, fmt.Errorf("invalid flatpak dependency format %q, must be remote:ref", d)
		}
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid flatpak dependency format %q, must be remote:ref", d)
		}
		out = append(out, config.FlatpakDep{
			Remote: parts[0],
			Ref:    parts[1],
		})
	}
	return out, nil
}

var nonAlphaNumRegexp = regexp.MustCompile(`[^a-z0-9_-]`)

func sanitizeRemoteName(name string) string {
	name = strings.ToLower(name)
	return nonAlphaNumRegexp.ReplaceAllString(name, "-")
}

// SplitAndCleanSlice splits each string in the slice by comma and newline,
// and trims whitespace around each resulting element.
func SplitAndCleanSlice(slice []string) []string {
	var cleaned []string
	for _, s := range slice {
		s = strings.ReplaceAll(s, "\r", "")
		s = strings.ReplaceAll(s, "\n", ",")
		parts := strings.Split(s, ",")
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
	}
	return cleaned
}

// parseAppIDRef parses an app-id reference of the format "app-id//branch".
// It returns the clean app ID and optionally the branch name if present.
func parseAppIDRef(ref string) (string, string) {
	if parts := strings.SplitN(ref, "//", 2); len(parts) == 2 {
		return parts[0], parts[1]
	}
	return ref, ""
}

// resolveBuildOptions consolidates the configuration lookup and CLI overrides
// logic for builder.BuildOptions used by both build and release commands.
func resolveBuildOptions(
	cmd *cobra.Command,
	cfg *config.Config,
	appConfig *config.App,
	appID string,
	manifest string,
	arch string,
	branch string,
	repoPath string,
) (builder.BuildOptions, error) {
	// 1. Initialize defaults
	var appCCacheDir = ".ccache"
	var appStateDir = ".state"
	var appRunLinter = false
	var appLinterStrict = true
	var appLinterIgnoreRules []string
	var appLinterExceptions []string
	var appLinterExceptionsFile = ""
	var appBuilderArgs []string
	var appRemotes map[string]config.RemoteConfig
	var appFlatpaks []config.FlatpakDep
	var appNoSign = false
	var appNoInstallDeps = false
	var appNoFlathub = false

	if cfg != nil {
		appNoSign = cfg.NoSign
	}

	// 2. Resolve defaults from app configuration
	if appConfig != nil {
		appCCacheDir = appConfig.CCacheDir
		appStateDir = appConfig.StateDir
		appRunLinter = appConfig.RunLinter
		appBuilderArgs = appConfig.BuilderArgs
		appRemotes = appConfig.Remotes
		appFlatpaks = appConfig.Flatpaks
		if appConfig.NoInstallDeps != nil {
			appNoInstallDeps = *appConfig.NoInstallDeps
		}
		if appConfig.NoFlathub != nil {
			appNoFlathub = *appConfig.NoFlathub
		}
		if appConfig.Linter != nil {
			if appConfig.Linter.Strict != nil {
				appLinterStrict = *appConfig.Linter.Strict
			}
			appLinterIgnoreRules = appConfig.Linter.IgnoreRules
			appLinterExceptions = appConfig.Linter.Exceptions
			appLinterExceptionsFile = appConfig.Linter.ExceptionsFile
		}
		if appConfig.CCache != nil && !*appConfig.CCache {
			appCCacheDir = ""
		}
	} else if cfg != nil {
		if cfg.Defaults != nil {
			appCCacheDir = cfg.Defaults.CCacheDir
			if appCCacheDir == "" {
				appCCacheDir = ".ccache"
			}
			appStateDir = cfg.Defaults.StateDir
			if appStateDir == "" {
				appStateDir = ".state"
			}
			appRunLinter = cfg.Defaults.RunLinter
			appBuilderArgs = cfg.Defaults.BuilderArgs
			appRemotes = cfg.Defaults.Remotes
			appFlatpaks = cfg.Defaults.Flatpaks
			if cfg.Defaults.NoInstallDeps != nil {
				appNoInstallDeps = *cfg.Defaults.NoInstallDeps
			}
			if cfg.Defaults.NoFlathub != nil {
				appNoFlathub = *cfg.Defaults.NoFlathub
			}
			if cfg.Defaults.CCache != nil && !*cfg.Defaults.CCache {
				appCCacheDir = ""
			}
		}
		if cfg.Linter != nil {
			if cfg.Linter.Strict != nil {
				appLinterStrict = *cfg.Linter.Strict
			}
			appLinterIgnoreRules = cfg.Linter.IgnoreRules
			appLinterExceptions = cfg.Linter.Exceptions
			appLinterExceptionsFile = cfg.Linter.ExceptionsFile
		}
	}

	// 3. Resolve CLI overrides
	if cmd.Flags().Changed("ccache-dir") {
		appCCacheDir, _ = cmd.Flags().GetString("ccache-dir")
	}
	if cmd.Flags().Changed("state-dir") {
		appStateDir, _ = cmd.Flags().GetString("state-dir")
	}
	if cmd.Flags().Changed("run-linter") {
		appRunLinter, _ = cmd.Flags().GetBool("run-linter")
	}
	if cmd.Flags().Lookup("no-sign") != nil && cmd.Flags().Changed("no-sign") {
		appNoSign, _ = cmd.Flags().GetBool("no-sign")
	}
	if cmd.Flags().Lookup("no-install-deps") != nil && cmd.Flags().Changed("no-install-deps") {
		appNoInstallDeps, _ = cmd.Flags().GetBool("no-install-deps")
	}
	if cmd.Flags().Lookup("no-flathub") != nil && cmd.Flags().Changed("no-flathub") {
		appNoFlathub, _ = cmd.Flags().GetBool("no-flathub")
	}
	if cmd.Flags().Changed("builder-arg") {
		appBuilderArgs, _ = cmd.Flags().GetStringSlice("builder-arg")
	}
	if cmd.Flags().Changed("flatpak-remote") {
		remotesList, _ := cmd.Flags().GetStringArray("flatpak-remote")
		parsed, err := parseFlatpakRemotes(remotesList)
		if err != nil {
			return builder.BuildOptions{}, err
		}
		appRemotes = parsed
	}
	if cmd.Flags().Changed("flatpak-dep") {
		depsList, _ := cmd.Flags().GetStringArray("flatpak-dep")
		parsed, err := parseFlatpakDeps(depsList)
		if err != nil {
			return builder.BuildOptions{}, err
		}
		appFlatpaks = parsed
	}

	var rawExceptionsFile string
	var rawExceptions []string
	if cmd.Flags().Changed("linter-exceptions-file") {
		rawExceptionsFile, _ = cmd.Flags().GetString("linter-exceptions-file")
	}
	if cmd.Flags().Changed("linter-exception") {
		rawExceptions, _ = cmd.Flags().GetStringSlice("linter-exception")
	}

	appLinterExceptions, appLinterExceptionsFile = resolveLinterExceptions(
		cmd.Flags().Changed("linter-exceptions-file"),
		cmd.Flags().Changed("linter-exception"),
		appLinterExceptions,
		appLinterExceptionsFile,
		rawExceptions,
		rawExceptionsFile,
	)

	// Build the final options
	var buildInstall bool
	if cmd.Flags().Lookup("install") != nil {
		buildInstall, _ = cmd.Flags().GetBool("install")
	}
	var buildBundle bool
	if cmd.Flags().Lookup("bundle") != nil {
		buildBundle, _ = cmd.Flags().GetBool("bundle")
	}

	opts := builder.BuildOptions{
		AppID:                appID,
		Manifest:             manifest,
		Arch:                 arch,
		Branch:               branch,
		CCacheDir:            appCCacheDir,
		StateDir:             appStateDir,
		RepoPath:             repoPath,
		RunLinter:            appRunLinter,
		LinterStrict:         appLinterStrict,
		LinterIgnoreRules:    appLinterIgnoreRules,
		LinterExceptions:     appLinterExceptions,
		LinterExceptionsFile: appLinterExceptionsFile,
		BuilderArgs:          appBuilderArgs,
		Remotes:              appRemotes,
		Flatpaks:             appFlatpaks,
		NoSign:               appNoSign,
		Install:              buildInstall,
		Bundle:               buildBundle,
		NoInstallDeps:        appNoInstallDeps,
		NoFlathub:            appNoFlathub,
	}

	return opts, nil
}
