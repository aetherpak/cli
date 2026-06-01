package cmd

import (
	"fmt"

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
	importBundleURL    string
	importBundleSHA256 string
	importBundlePath   string
	importRepoPath     string
	importOutputFile   string
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Ingests external Flatpak bundles and rebinds branches",
	Long:  `Downloads or processes a local Flatpak bundle (.flatpak), verifies its checksum, and rebinds its branch metadata to match the target channel.`,
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

		type importJob struct {
			appID        string
			arch         string
			branch       string
			bundleURL    string
			bundleSHA256 string
			bundlePath   string
		}

		var jobs []importJob

		explicitBundle := (importBundleURL != "" || importBundlePath != "")

		if explicitBundle {
			// Explicit bundle always imports just this one
			jobs = append(jobs, importJob{
				appID:        importAppID,
				arch:         importArch,
				branch:       importBranch,
				bundleURL:    importBundleURL,
				bundleSHA256: importBundleSHA256,
				bundlePath:   importBundlePath,
			})
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

		for _, job := range jobs {
			opts := importer.ImportOptions{
				AppID:        job.appID,
				Arch:         job.arch,
				Branch:       job.branch,
				BundleURL:    job.bundleURL,
				BundleSHA256: job.bundleSHA256,
				BundlePath:   job.bundlePath,
				RepoPath:     importRepoPath,
			}

			if err := importer.Import(opts); err != nil {
				return NewCmdError(1, err)
			}

			repoPath := importRepoPath
			if repoPath == "" {
				repoPath = "repo"
			}
			info, err := repoinfo.Resolve(repoPath)
			if err != nil {
				return NewCmdErrorf(1, "imported repo has no resolvable ref: %w", err)
			}
			if err := ciout.Emit(importOutputFile, []ciout.KV{
				{Key: "app-id", Value: info.AppID},
				{Key: "branch", Value: info.Branch},
				{Key: "arch", Value: info.Arch},
				{Key: "repo-path", Value: repoPath},
			}); err != nil {
				return NewCmdError(1, err)
			}
			logger.SuccessBanner("Import Completed", fmt.Sprintf("Successfully imported application %s (%s) for channel %s.", info.AppID, info.Arch, info.Branch))
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
	importCmd.Flags().StringVar(&importBundleURL, "bundle-url", "", "HTTP URL of the remote bundle")
	importCmd.Flags().StringVar(&importBundleSHA256, "bundle-sha256", "", "expected SHA-256 checksum of the bundle")
	importCmd.Flags().StringVar(&importBundlePath, "bundle-path", "", "local path override to Flatpak bundle file")
	importCmd.Flags().StringVar(&importRepoPath, "repo-path", "repo", "destination OSTree repository path")
	importCmd.Flags().StringVar(&importOutputFile, "output-file", "", "write resolved outputs as dotenv KEY=VALUE (- or empty = stdout)")
}
