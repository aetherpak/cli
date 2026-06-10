package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/builder"
	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/manifest"
	"github.com/aetherpak/aetherpak/pkg/repoinfo"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	buildAppID                string
	buildManifest             string
	buildArch                 string
	buildBranch               string
	buildCCacheDir            string
	buildStateDir             string
	buildRepoPath             string
	buildOutputFile           string
	buildBuilderArgs          []string
	buildRunLinter            bool
	buildLinterExceptionsFile string
	buildLinterExceptions     []string
	buildFlatpakRemotes       []string
	buildFlatpakDeps          []string
	buildNoSign               bool
	buildInstall              bool
	buildBundle               bool
	buildNoInstallDeps        bool
	buildNoFlathub            bool
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Executes flatpak-builder compilation sandbox",
	Long:  `Invokes the flatpak-builder tool to compile and export the manifest application into a local OSTree repo.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		hasConfig := true
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}
		if viper.ConfigFileUsed() == "" {
			hasConfig = false
		}

		if err := config.ValidateArch(buildArch); err != nil {
			return NewCmdError(2, err)
		}

		repoPath := buildRepoPath
		if !cmd.Flags().Changed("repo-path") && cfg.OutputDir != "" {
			repoPath = filepath.Join(cfg.OutputDir, "repo")
		} else if repoPath == "" {
			repoPath = "repo"
		}

		var buildForceBranch string
		if buildAppID != "" {
			cleanID, br := parseAppIDRef(buildAppID)
			buildAppID = cleanID
			buildForceBranch = br
		}

		manifestSet := cmd.Flags().Changed("manifest")

		type buildJob struct {
			appID     string
			manifest  string
			branch    string
			appConfig *config.App
		}

		var jobs []buildJob

		if manifestSet {
			// Explicit manifest always builds just this one
			jobs = append(jobs, buildJob{
				appID:    buildAppID,
				manifest: buildManifest,
				branch:   buildBranch,
			})
			// Find matching app config if any (to apply options)
			if buildAppID != "" && len(cfg.Apps) > 0 {
				for i := range cfg.Apps {
					if cfg.Apps[i].ID == buildAppID {
						jobs[0].appConfig = &cfg.Apps[i]
						break
					}
				}
			}
		} else if buildAppID != "" {
			// Explicit app ID
			var appConfig *config.App
			for i := range cfg.Apps {
				if cfg.Apps[i].ID == buildAppID {
					appConfig = &cfg.Apps[i]
					break
				}
			}
			if appConfig == nil {
				return NewCmdErrorf(1, "app %q not found in config", buildAppID)
			}
			if appConfig.Manifest == "" {
				return NewCmdErrorf(1, "app %q has no manifest configured", buildAppID)
			}
			jobs = append(jobs, buildJob{
				appID:     appConfig.ID,
				manifest:  appConfig.Manifest,
				branch:    appConfig.Branch,
				appConfig: appConfig,
			})
		} else {
			// No app-id and no manifest:
			if !hasConfig {
				return NewCmdError(2, fmt.Errorf("no manifest provided and no configuration file found"))
			}
			// Gather all apps with manifests
			for i := range cfg.Apps {
				if cfg.Apps[i].Manifest != "" {
					jobs = append(jobs, buildJob{
						appID:     cfg.Apps[i].ID,
						manifest:  cfg.Apps[i].Manifest,
						branch:    cfg.Apps[i].Branch,
						appConfig: &cfg.Apps[i],
					})
				}
			}
			if len(jobs) == 0 {
				return NewCmdError(2, fmt.Errorf("no applications with manifest configurations found in configuration file"))
			}
		}

		for _, job := range jobs {
			jobBranch := buildBranch
			if jobBranch == "" {
				jobBranch = buildForceBranch
			}
			if jobBranch == "" {
				jobBranch = job.branch
			}
			if jobBranch == "" {
				if ch := resolveChannelFromEnv(); ch != "" {
					jobBranch = ch
				} else {
					jobBranch = "stable"
				}
			}

			// Resolve build option defaults from configuration
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

			if job.appConfig != nil {
				appCCacheDir = job.appConfig.CCacheDir
				appStateDir = job.appConfig.StateDir
				appRunLinter = job.appConfig.RunLinter
				appBuilderArgs = job.appConfig.BuilderArgs
				appRemotes = job.appConfig.Remotes
				appFlatpaks = job.appConfig.Flatpaks
				if job.appConfig.NoInstallDeps != nil {
					appNoInstallDeps = *job.appConfig.NoInstallDeps
				}
				if job.appConfig.NoFlathub != nil {
					appNoFlathub = *job.appConfig.NoFlathub
				}
				if job.appConfig.Linter != nil {
					if job.appConfig.Linter.Strict != nil {
						appLinterStrict = *job.appConfig.Linter.Strict
					}
					appLinterIgnoreRules = job.appConfig.Linter.IgnoreRules
					appLinterExceptions = job.appConfig.Linter.Exceptions
					appLinterExceptionsFile = job.appConfig.Linter.ExceptionsFile
				}
				if job.appConfig.CCache != nil && !*job.appConfig.CCache {
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
			if cmd.Flags().Changed("no-sign") {
				appNoSign = buildNoSign
			}
			if cmd.Flags().Changed("no-install-deps") {
				appNoInstallDeps = buildNoInstallDeps
			}
			if cmd.Flags().Changed("no-flathub") {
				appNoFlathub = buildNoFlathub
			}
			if cmd.Flags().Changed("builder-arg") {
				appBuilderArgs = buildBuilderArgs
			}
			if cmd.Flags().Changed("flatpak-remote") {
				parsed, err := parseFlatpakRemotes(buildFlatpakRemotes)
				if err != nil {
					return NewCmdError(2, err)
				}
				appRemotes = parsed
			}
			if cmd.Flags().Changed("flatpak-dep") {
				parsed, err := parseFlatpakDeps(buildFlatpakDeps)
				if err != nil {
					return NewCmdError(2, err)
				}
				appFlatpaks = parsed
			}

			appLinterExceptions, appLinterExceptionsFile = resolveLinterExceptions(
				cmd.Flags().Changed("linter-exceptions-file"),
				cmd.Flags().Changed("linter-exception"),
				appLinterExceptions,
				appLinterExceptionsFile,
				buildLinterExceptions,
				buildLinterExceptionsFile,
			)

			opts := builder.BuildOptions{
				AppID:                job.appID,
				Manifest:             job.manifest,
				Arch:                 buildArch,
				Branch:               jobBranch,
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

			if err := builder.Build(opts); err != nil {
				return NewCmdError(1, err)
			}

			// Prefer repo's ref for resolved coordinates; fallback to requested values.
			resolvedAppID, resolvedBranch, resolvedArch := job.appID, jobBranch, buildArch
			if info, err := repoinfo.Resolve(repoPath); err == nil {
				resolvedAppID, resolvedBranch, resolvedArch = info.AppID, info.Branch, info.Arch
			}
			kvList := []ciout.KV{
				{Key: "app-id", Value: resolvedAppID},
				{Key: "branch", Value: resolvedBranch},
				{Key: "arch", Value: resolvedArch},
				{Key: "repo-path", Value: repoPath},
			}
			if buildBundle {
				var extensionIDs []string
				if m, err := manifest.ParseManifest(job.manifest); err == nil {
					extensionIDs = m.ExtensionIDs
				}

				bundleDir := filepath.Dir(repoPath)
				var bundlePaths []string
				var mainBundleFile string

				if refs, err := repoinfo.ResolveAll(repoPath); err == nil {
					for _, ref := range refs {
						if manifest.IsRefRelated(ref.AppID, resolvedAppID, extensionIDs) {
							bundleFile := filepath.Join(bundleDir, ref.AppID+".flatpak")
							bundlePaths = append(bundlePaths, bundleFile)
							if ref.AppID == resolvedAppID {
								mainBundleFile = bundleFile
							}
						}
					}
				}

				if mainBundleFile == "" {
					mainBundleFile = filepath.Join(bundleDir, resolvedAppID+".flatpak")
					if len(bundlePaths) == 0 {
						bundlePaths = append(bundlePaths, mainBundleFile)
					}
				}

				kvList = append(kvList, ciout.KV{Key: "bundle-path", Value: mainBundleFile})
				if len(bundlePaths) > 0 {
					kvList = append(kvList, ciout.KV{Key: "bundle-paths", Value: strings.Join(bundlePaths, ",")})
				}
			}
			if err := ciout.Emit(buildOutputFile, kvList); err != nil {
				return NewCmdError(1, err)
			}
			logger.SuccessBanner("Build Completed", fmt.Sprintf("Successfully built application %s (%s) for channel %s.", resolvedAppID, resolvedArch, resolvedBranch))
		}

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
	buildCmd.Flags().StringVar(&buildLinterExceptionsFile, "linter-exceptions-file", "", "path to linter exceptions file (JSON)")
	buildCmd.Flags().StringSliceVar(&buildLinterExceptions, "linter-exception", nil, "linter exceptions to ignore")
	buildCmd.Flags().StringArrayVar(&buildFlatpakRemotes, "flatpak-remote", nil, "Flatpak remote repository to register (format: name=url)")
	buildCmd.Flags().StringArrayVar(&buildFlatpakDeps, "flatpak-dep", nil, "Flatpak dependency to install before build (format: remote:ref)")
	buildCmd.Flags().BoolVar(&buildNoSign, "no-sign", false, "disable GPG verification/signing")
	buildCmd.Flags().BoolVar(&buildInstall, "install", false, "install application after build")
	buildCmd.Flags().BoolVar(&buildBundle, "bundle", false, "generate a bundled flatpak binary (.flatpak) for the application")
	buildCmd.Flags().BoolVar(&buildNoInstallDeps, "no-install-deps", false, "disable auto-injection of --install-deps-from flags for remotes")
	buildCmd.Flags().BoolVar(&buildNoFlathub, "no-flathub", false, "disable auto-injection of flathub as a dependency remote")
}
