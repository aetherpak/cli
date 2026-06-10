package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/builder"
	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/executil"
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
	RunE:  runBuild,
}

func runBuild(cmd *cobra.Command, args []string) error {
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

	executor := executil.NewOSExecutor()

	for _, job := range jobs {
		jobBranch := buildBranch
		if jobBranch == "" {
			jobBranch = buildForceBranch
		}
		if jobBranch == "" {
			jobBranch = job.branch
		}
		if jobBranch == "" {
			if ch := resolveChannelFromEnv(cfg); ch != "" {
				jobBranch = ch
			} else {
				jobBranch = "stable"
			}
		}

		opts, err := resolveBuildOptions(
			cmd,
			cfg,
			job.appConfig,
			job.appID,
			job.manifest,
			buildArch,
			jobBranch,
			repoPath,
		)
		if err != nil {
			return NewCmdError(2, err)
		}
		// Inject executor explicitly
		opts.Executor = executor

		if err := builder.Build(opts); err != nil {
			return NewCmdError(1, err)
		}

		// Prefer repo's ref for resolved coordinates; fallback to requested values.
		resolvedAppID, resolvedBranch, resolvedArch := job.appID, jobBranch, buildArch
		if info, err := repoinfo.Resolve(executor, repoPath); err == nil {
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

			if refs, err := repoinfo.ResolveAll(executor, repoPath); err == nil {
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
