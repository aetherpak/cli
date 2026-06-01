package cmd

import (
	"fmt"

	"github.com/aetherpak/aetherpak/pkg/builder"
	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/repoinfo"
	"github.com/spf13/cobra"
)

var (
	buildAppID       string
	buildManifest    string
	buildArch        string
	buildBranch      string
	buildCCacheDir   string
	buildStateDir    string
	buildRepoPath    string
	buildOutputFile  string
	buildBuilderArgs []string
	buildRunLinter   bool
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Executes flatpak-builder compilation sandbox",
	Long:  `Invokes the flatpak-builder tool to compile and export the manifest application into a local OSTree repo.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}

		// An explicit --manifest always wins; config only supplies the manifest
		// when the caller named an app (or none) without one.
		manifestSet := cmd.Flags().Changed("manifest")
		var appConfig *config.App
		if len(cfg.Apps) > 0 {
			if buildAppID != "" {
				for i, app := range cfg.Apps {
					if app.ID == buildAppID {
						if !manifestSet {
							buildManifest = app.Manifest
						}
						if buildBranch == "" {
							buildBranch = app.Branch
						}
						appConfig = &cfg.Apps[i]
						break
					}
				}
			} else if !manifestSet {
				first := cfg.Apps[0]
				buildAppID = first.ID
				buildManifest = first.Manifest
				if buildBranch == "" {
					buildBranch = first.Branch
				}
				appConfig = &cfg.Apps[0]
			}
		}

		if buildManifest == "" {
			return NewCmdError(2, fmt.Errorf("manifest is required (either via flag or config file)"))
		}

		if buildArch == "" {
			buildArch = "x86_64"
		}
		if buildBranch == "" {
			if ch := resolveChannelFromEnv(); ch != "" {
				buildBranch = ch
			} else {
				buildBranch = "stable"
			}
		}

		// Resolve build option defaults from configuration
		var appCCacheDir = ".ccache"
		var appStateDir = ".state"
		var appRunLinter = false
		var appLinterStrict = true
		var appLinterIgnoreRules []string
		var appBuilderArgs []string

		if cfg != nil {
			if appConfig != nil {
				appCCacheDir = appConfig.CCacheDir
				appStateDir = appConfig.StateDir
				appRunLinter = appConfig.RunLinter
				appBuilderArgs = appConfig.BuilderArgs
				if appConfig.Linter != nil {
					if appConfig.Linter.Strict != nil {
						appLinterStrict = *appConfig.Linter.Strict
					}
					appLinterIgnoreRules = appConfig.Linter.IgnoreRules
				}
				if appConfig.CCache != nil && !*appConfig.CCache {
					appCCacheDir = ""
				}
			} else {
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
					if cfg.Defaults.CCache != nil && !*cfg.Defaults.CCache {
						appCCacheDir = ""
					}
				}
				if cfg.Linter != nil {
					if cfg.Linter.Strict != nil {
						appLinterStrict = *cfg.Linter.Strict
					}
					appLinterIgnoreRules = cfg.Linter.IgnoreRules
				}
			}
		}

		// Apply CLI flag overrides if explicitly passed
		if cmd.Flags().Changed("ccache-dir") {
			appCCacheDir = buildCCacheDir
		}
		if cmd.Flags().Changed("state-dir") {
			appStateDir = buildStateDir
		}
		if cmd.Flags().Changed("run-linter") {
			appRunLinter = buildRunLinter
		}
		if cmd.Flags().Changed("builder-arg") {
			appBuilderArgs = buildBuilderArgs
		}

		opts := builder.BuildOptions{
			AppID:             buildAppID,
			Manifest:          buildManifest,
			Arch:              buildArch,
			Branch:            buildBranch,
			CCacheDir:         appCCacheDir,
			StateDir:          appStateDir,
			RepoPath:          buildRepoPath,
			RunLinter:         appRunLinter,
			LinterStrict:      appLinterStrict,
			LinterIgnoreRules: appLinterIgnoreRules,
			BuilderArgs:       appBuilderArgs,
		}

		if err := builder.Build(opts); err != nil {
			return NewCmdError(1, err)
		}

		repoPath := buildRepoPath
		if repoPath == "" {
			repoPath = "repo"
		}
		// Prefer repo's ref for resolved coordinates; fallback to requested values.
		appID, branch, arch := buildAppID, buildBranch, buildArch
		if info, err := repoinfo.Resolve(repoPath); err == nil {
			appID, branch, arch = info.AppID, info.Branch, info.Arch
		}
		if err := ciout.Emit(buildOutputFile, []ciout.KV{
			{Key: "app-id", Value: appID},
			{Key: "branch", Value: branch},
			{Key: "arch", Value: arch},
			{Key: "repo-path", Value: repoPath},
		}); err != nil {
			return NewCmdError(1, err)
		}
		logger.SuccessBanner("Build Completed", fmt.Sprintf("Successfully built application %s (%s) for channel %s.", appID, arch, branch))
		return nil
	},
}

func init() {
	RootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringVar(&buildAppID, "app-id", "", "app ID (reverse-DNS format)")
	buildCmd.Flags().StringVar(&buildAppID, "app", "", "deprecated alias for --app-id")
	_ = buildCmd.Flags().MarkDeprecated("app", "please use --app-id instead")
	buildCmd.Flags().StringVar(&buildManifest, "manifest", "", "path to manifest file")
	buildCmd.Flags().StringVar(&buildArch, "arch", "x86_64", "target CPU architecture")
	buildCmd.Flags().StringVar(&buildBranch, "branch", "", "published branch channel")
	buildCmd.Flags().StringVar(&buildCCacheDir, "ccache-dir", ".ccache", "ccache directory")
	buildCmd.Flags().StringVar(&buildStateDir, "state-dir", ".state", "builder state directory")
	buildCmd.Flags().StringVar(&buildRepoPath, "repo-path", "repo", "destination OSTree repository path")
	buildCmd.Flags().StringVar(&buildOutputFile, "output-file", "", "write resolved outputs as dotenv KEY=VALUE (- or empty = stdout)")
	buildCmd.Flags().StringArrayVar(&buildBuilderArgs, "builder-arg", nil, "extra argument passed through to flatpak-builder")
	buildCmd.Flags().BoolVar(&buildRunLinter, "run-linter", false, "run flatpak-builder-lint before and after build")
}
