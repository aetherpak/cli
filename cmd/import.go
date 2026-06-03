package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/importer"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/repoinfo"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	importAppID        string
	importArch         string
	importBranch       string
	importBundleURLs   []string
	importBundleSHA256 string
	importBundlePaths  []string
	importRepoPath     string
	importOutputFile   string
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Ingests external Flatpak bundles and rebinds branches",
	Long:  `Downloads or processes local Flatpak bundles (.flatpak), verifies their checksums, and rebinds branch metadata to match the target channel.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		hasConfig := true
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}
		if viper.ConfigFileUsed() == "" {
			hasConfig = false
		}

		if err := config.ValidateArch(importArch); err != nil {
			return NewCmdError(2, err)
		}

		repoPath := importRepoPath
		if !cmd.Flags().Changed("repo-path") && cfg.OutputDir != "" {
			repoPath = filepath.Join(cfg.OutputDir, "repo")
		} else if repoPath == "" {
			repoPath = "repo"
		}

		// Expand glob patterns for paths
		var resolvedPaths []string
		for _, pat := range importBundlePaths {
			matches, err := filepath.Glob(pat)
			if err != nil {
				matches = []string{pat}
			}
			if len(matches) == 0 {
				matches = []string{pat}
			}
			resolvedPaths = append(resolvedPaths, matches...)
		}

		totalBundles := len(importBundleURLs) + len(resolvedPaths)
		explicitBundle := totalBundles > 0

		if totalBundles > 1 {
			if importBundleSHA256 != "" {
				return NewCmdError(2, fmt.Errorf("cannot specify --bundle-sha256 when importing multiple bundles"))
			}
			if importAppID != "" {
				return NewCmdError(2, fmt.Errorf("cannot specify --app-id when importing multiple bundles; coordinates must be auto-detected from each bundle's internal metadata"))
			}
			if cmd.Flags().Changed("arch") {
				return NewCmdError(2, fmt.Errorf("cannot specify --arch when importing multiple bundles; coordinates must be auto-detected from each bundle's internal metadata"))
			}
		}

		type importJob struct {
			appID        string
			arch         string
			branch       string
			bundleURL    string
			bundleSHA256 string
			bundlePath   string
		}

		var jobs []importJob

		if explicitBundle {
			// Explicit bundles specified
			for _, url := range importBundleURLs {
				jobs = append(jobs, importJob{
					appID:        importAppID,
					arch:         importArch,
					branch:       importBranch,
					bundleURL:    url,
					bundleSHA256: importBundleSHA256,
				})
			}
			for _, path := range resolvedPaths {
				jobs = append(jobs, importJob{
					appID:      importAppID,
					arch:       importArch,
					branch:     importBranch,
					bundlePath: path,
				})
			}
		} else if importAppID != "" {
			// Explicit app ID, find in config
			var targetApp *config.App
			for i := range cfg.Apps {
				if cfg.Apps[i].ID == importAppID {
					targetApp = &cfg.Apps[i]
					break
				}
			}
			if targetApp == nil {
				return NewCmdErrorf(1, "app %q not found in config", importAppID)
			}
			arch := importArch
			if arch == "" {
				arch = "x86_64"
			}
			bundle, exists := targetApp.Bundles[arch]
			if !exists {
				return NewCmdErrorf(1, "no bundle configured for architecture %q for app %s", arch, targetApp.ID)
			}
			branch := importBranch
			if branch == "" {
				branch = targetApp.Branch
			}
			jobs = append(jobs, importJob{
				appID:        targetApp.ID,
				arch:         arch,
				branch:       branch,
				bundleURL:    bundle.URL,
				bundleSHA256: bundle.SHA256,
			})
		} else {
			// No app-id and no bundle specified:
			if !hasConfig {
				return NewCmdError(2, fmt.Errorf("either bundle-url or bundle-path is required"))
			}
			// Gather all apps with bundle configs for the target arch
			arch := importArch
			if arch == "" {
				arch = "x86_64" // fallback default for multi-app if not specified
			}
			for i := range cfg.Apps {
				if len(cfg.Apps[i].Bundles) > 0 {
					if bundle, exists := cfg.Apps[i].Bundles[arch]; exists {
						branch := importBranch
						if branch == "" {
							branch = cfg.Apps[i].Branch
						}
						jobs = append(jobs, importJob{
							appID:        cfg.Apps[i].ID,
							arch:         arch,
							branch:       branch,
							bundleURL:    bundle.URL,
							bundleSHA256: bundle.SHA256,
						})
					}
				}
			}
			if len(jobs) == 0 {
				return NewCmdError(2, fmt.Errorf("no applications with bundle configurations for architecture %q found in configuration file", arch))
			}
		}

		var tempRepoDir string
		var useTempRepo bool

		if importAppID != "" && cmd.Flags().Changed("arch") && importBranch != "" {
			useTempRepo = false
		} else {
			useTempRepo = true
			var err error
			tempRepoDir, err = os.MkdirTemp("", "aetherpak-import-temp-*")
			if err != nil {
				return NewCmdErrorf(1, "failed to create temp repo directory: %w", err)
			}
			defer os.RemoveAll(tempRepoDir)
		}

		destRepo := repoPath
		importRepo := destRepo
		if useTempRepo {
			importRepo = tempRepoDir
		}

		for _, job := range jobs {
			opts := importer.ImportOptions{
				AppID:        job.appID,
				Arch:         job.arch,
				Branch:       job.branch,
				BundleURL:    job.bundleURL,
				BundleSHA256: job.bundleSHA256,
				BundlePath:   job.bundlePath,
				RepoPath:     importRepo,
			}

			if err := importer.Import(opts); err != nil {
				return NewCmdError(1, err)
			}
		}

		var resolvedApps []repoinfo.Info
		if useTempRepo {
			infos, err := repoinfo.ResolveAll(tempRepoDir)
			if err != nil {
				return NewCmdErrorf(1, "failed to resolve imported bundle refs: %w", err)
			}
			resolvedApps = infos
		} else {
			resolvedApps = []repoinfo.Info{{
				AppID:    importAppID,
				Arch:     importArch,
				Branch:   importBranch,
				RepoPath: destRepo,
			}}
		}

		if useTempRepo {
			// Copy from tempRepoDir to destRepo
			if err := os.MkdirAll(destRepo, 0755); err != nil {
				return fmt.Errorf("failed to create target repo directory: %w", err)
			}
			if _, err := os.Stat(filepath.Join(destRepo, "config")); os.IsNotExist(err) {
				initCmd := exec.Command("ostree", "--repo="+destRepo, "init", "--mode=archive-z2")
				if err := initCmd.Run(); err != nil {
					return fmt.Errorf("failed to initialize target ostree repo: %w", err)
				}
			}

			for _, ra := range resolvedApps {
				destRef := fmt.Sprintf("app/%s/%s/%s", ra.AppID, ra.Arch, ra.Branch)
				logger.Info("Copying ref %s from temp repo to target repo...", destRef)
				copyCmd := exec.Command("flatpak", "build-commit-from",
					"--src-repo="+tempRepoDir,
					"--src-ref="+destRef,
					"--update-appstream",
					"--no-update-summary",
					destRepo,
					destRef,
				)
				var copyStderr bytes.Buffer
				copyCmd.Stderr = &copyStderr
				if err := copyCmd.Run(); err != nil {
					return fmt.Errorf("failed to copy commit to target repository (%w): %s", err, copyStderr.String())
				}
			}
		}

		for _, ra := range resolvedApps {
			if err := ciout.Emit(importOutputFile, []ciout.KV{
				{Key: "app-id", Value: ra.AppID},
				{Key: "branch", Value: ra.Branch},
				{Key: "arch", Value: ra.Arch},
				{Key: "repo-path", Value: destRepo},
			}); err != nil {
				return NewCmdError(1, err)
			}
			logger.SuccessBanner("Import Completed", fmt.Sprintf("Successfully imported application %s (%s) for channel %s.", ra.AppID, ra.Arch, ra.Branch))
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(importCmd)

	importCmd.Flags().StringVar(&importAppID, "app-id", "", "app ID (reverse-DNS format); derived from the bundle when empty")
	importCmd.Flags().StringVar(&importAppID, "app", "", "deprecated alias for --app-id")
	_ = importCmd.Flags().MarkDeprecated("app", "please use --app-id instead")
	importCmd.Flags().StringVar(&importArch, "arch", "", "target CPU architecture; derived from the bundle when empty")
	importCmd.Flags().StringVar(&importBranch, "branch", "", "published branch channel; derived from the bundle when empty")
	importCmd.Flags().StringSliceVar(&importBundleURLs, "bundle-url", nil, "HTTP URL(s) of the remote bundle(s)")
	importCmd.Flags().StringVar(&importBundleSHA256, "bundle-sha256", "", "expected SHA-256 checksum of the bundle")
	importCmd.Flags().StringSliceVar(&importBundlePaths, "bundle-path", nil, "local path(s) override to Flatpak bundle file(s) (supports globs)")
	importCmd.Flags().StringVar(&importRepoPath, "repo-path", "repo", "destination OSTree repository path")
	importCmd.Flags().StringVar(&importOutputFile, "output-file", "", "write resolved outputs as dotenv KEY=VALUE (- or empty = stdout)")
}
