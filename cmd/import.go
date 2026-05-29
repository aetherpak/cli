package cmd

import (
	"fmt"
	"os"

	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/importer"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/repoinfo"
	"github.com/spf13/cobra"
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
	Run: func(cmd *cobra.Command, args []string) {
		// Resolve the bundle source from config only when none was given
		// explicitly; an explicit --bundle-url/--bundle-path always wins.
		if importBundleURL == "" && importBundlePath == "" {
			cfg, err := LoadConfig()
			if err == nil && len(cfg.Apps) > 0 {
				var matched bool
				for _, app := range cfg.Apps {
					if importAppID != "" && app.ID == importAppID {
						if len(app.Bundles) > 0 {
							// Determine target architecture
							arch := importArch
							if arch == "" {
								arch = "x86_64"
							}
							if bundle, exists := app.Bundles[arch]; exists {
								importBundleURL = bundle.URL
								importBundleSHA256 = bundle.SHA256
								if importBranch == "" {
									importBranch = app.Branch
								}
								matched = true
							}
						}
						break
					}
				}
				if !matched && importAppID == "" {
					// Fallback to first bundle app in config
					for _, app := range cfg.Apps {
						if len(app.Bundles) > 0 {
							importAppID = app.ID
							arch := importArch
							if arch == "" {
								arch = "x86_64"
							}
							if bundle, exists := app.Bundles[arch]; exists {
								importBundleURL = bundle.URL
								importBundleSHA256 = bundle.SHA256
								if importBranch == "" {
									importBranch = app.Branch
								}
								break
							}
						}
					}
				}
			}
		}

		if importBundleURL == "" && importBundlePath == "" {
			fmt.Fprintln(os.Stderr, "Error: either bundle-url or bundle-path is required")
			os.Exit(2)
		}

		// app-id, arch, and branch are optional: when unset they are derived
		// from the bundle's own app/<id>/<arch>/<branch> ref by the importer.

		opts := importer.ImportOptions{
			AppID:        importAppID,
			Arch:         importArch,
			Branch:       importBranch,
			BundleURL:    importBundleURL,
			BundleSHA256: importBundleSHA256,
			BundlePath:   importBundlePath,
			RepoPath:     importRepoPath,
		}

		if err := importer.Import(opts); err != nil {
			logger.ErrorBanner("Import Failed", err.Error())
			os.Exit(1)
		}

		repoPath := importRepoPath
		if repoPath == "" {
			repoPath = "repo"
		}
		info, err := repoinfo.Resolve(repoPath)
		if err != nil {
			logger.ErrorBanner("Import Failed", fmt.Sprintf("imported repo has no resolvable ref: %v", err))
			os.Exit(1)
		}
		if err := ciout.Emit(importOutputFile, []ciout.KV{
			{Key: "app-id", Value: info.AppID},
			{Key: "branch", Value: info.Branch},
			{Key: "arch", Value: info.Arch},
			{Key: "repo-path", Value: repoPath},
		}); err != nil {
			logger.ErrorBanner("Import Failed", err.Error())
			os.Exit(1)
		}
		logger.SuccessBanner("Import Completed", fmt.Sprintf("Successfully imported application %s (%s) for channel %s.", info.AppID, info.Arch, info.Branch))
	},
}

func init() {
	RootCmd.AddCommand(importCmd)

	importCmd.Flags().StringVar(&importAppID, "app", "", "app ID (reverse-DNS format); derived from the bundle when empty")
	importCmd.Flags().StringVar(&importArch, "arch", "", "target CPU architecture; derived from the bundle when empty")
	importCmd.Flags().StringVar(&importBranch, "branch", "", "published branch channel; derived from the bundle when empty")
	importCmd.Flags().StringVar(&importBundleURL, "bundle-url", "", "HTTP URL of the remote bundle")
	importCmd.Flags().StringVar(&importBundleSHA256, "bundle-sha256", "", "expected SHA-256 checksum of the bundle")
	importCmd.Flags().StringVar(&importBundlePath, "bundle-path", "", "local path override to Flatpak bundle file")
	importCmd.Flags().StringVar(&importRepoPath, "repo-path", "repo", "destination OSTree repository path")
	importCmd.Flags().StringVar(&importOutputFile, "output-file", "", "write resolved outputs as dotenv KEY=VALUE (- or empty = stdout)")
}
