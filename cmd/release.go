package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aetherpak/aetherpak/pkg/builder"
	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/importer"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/oci"
	"github.com/aetherpak/aetherpak/pkg/plan"
	"github.com/aetherpak/aetherpak/pkg/site"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

var (
	relBaseSHA       string
	relForce         string
	relWorkflowPath  string
	relGPGKeys       []string
	relGPGPassphrase string
	relInsecure      bool
	relRepoPath      string
	relCCacheDir     string
	relStateDir      string
	relRecordsDir    string
	relSiteDir       string
	relWorkers       int
	relRunLinter     bool
	relOutputFile    string
	relIndexTemplate string
	relNoSign        bool
	relAllowUnsigned bool

	relRemoteName           string
	relRuntimeRepo          string
	relRepoTitle            string
	relRepoHomepage         string
	relBuilderArgs          []string
	relReconcile            bool
	relLandingPage          bool
	relLinterExceptionsFile string
	relLinterExceptions     []string
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Runs plan, concurrent publish, and site index compilation",
	Long:  `Fully orchestrates the AetherPak lifecycle: plans matrix deltas, builds/imports changed packages concurrently, pushes OCI layers, and rebuilds Pages static site references.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}

		logger.Info("Phase 1: Planning release changes...")
		repoPath := relRepoPath
		if !cmd.Flags().Changed("repo-path") && cfg.OutputDir != "" {
			repoPath = filepath.Join(cfg.OutputDir, "repo")
		} else if repoPath == "" {
			repoPath = "repo"
		}

		recordsDir := relRecordsDir
		if !cmd.Flags().Changed("records-dir") && cfg.OutputDir != "" {
			recordsDir = filepath.Join(cfg.OutputDir, "records")
		} else if recordsDir == "" {
			recordsDir = "records"
		}

		siteDir := relSiteDir
		if !cmd.Flags().Changed("site-dir") && cfg.OutputDir != "" {
			siteDir = filepath.Join(cfg.OutputDir, "_site")
		} else if siteDir == "" {
			siteDir = "_site"
		}

		configPath := viper.ConfigFileUsed()
		if configPath == "" {
			if vCfgFile := viper.GetString("config"); vCfgFile != "" {
				configPath = vCfgFile
			} else {
				configPath = "aetherpak.yaml"
				if _, err := os.Stat("aetherpak.yml"); err == nil {
					configPath = "aetherpak.yml"
				}
			}
		}

		res, err := plan.ComputePlan(cfg, configPath, relBaseSHA, relForce, relWorkflowPath)
		if err != nil {
			return NewCmdErrorf(1, "Release planning failed: %w", err)
		}

		keys := relGPGKeys

		var passphrase []byte
		if relGPGPassphrase != "" {
			passphrase = []byte(relGPGPassphrase)
		}
		defer func() {
			if len(passphrase) > 0 {
				for i := range passphrase {
					passphrase[i] = 0
				}
			}
		}()

		noSign := relNoSign
		allowUnsigned := relAllowUnsigned

		ccacheDirChanged := cmd.Flags().Changed("ccache-dir")
		stateDirChanged := cmd.Flags().Changed("state-dir")
		runLinterChanged := cmd.Flags().Changed("run-linter")
		builderArgChanged := cmd.Flags().Changed("builder-arg")
		linterExceptionsFileChanged := cmd.Flags().Changed("linter-exceptions-file")
		linterExceptionChanged := cmd.Flags().Changed("linter-exception")

		if len(res.Matrix) == 0 {
			logger.Info("No application changes detected. Proceeding to site index update.")
		} else {
			logger.Info("Phase 2: Processing %d matrix rows concurrently (workers=%d)...", len(res.Matrix), relWorkers)

			// Spin up concurrent worker pool using errgroup
			g, _ := errgroup.WithContext(context.Background())
			rowChan := make(chan plan.MatrixRow, len(res.Matrix))

			// Seed matrix rows into worker queue
			for _, row := range res.Matrix {
				rowChan <- row
			}
			close(rowChan)

			// Spin up worker goroutines
			for i := 0; i < relWorkers; i++ {
				g.Go(func() error {
					for row := range rowChan {
						// 1. Build or Import
						if row.Source == "manifest" {
							var appCCacheDir = relCCacheDir
							var appStateDir = relStateDir
							var appRunLinter = row.RunLinter
							var appLinterStrict = true
							var appLinterIgnoreRules []string
							var appLinterExceptions []string
							var appLinterExceptionsFile = ""
							var appBuilderArgs []string

							var matchedApp *config.App
							for idx := range cfg.Apps {
								if cfg.Apps[idx].ID == row.AppID {
									matchedApp = &cfg.Apps[idx]
									break
								}
							}

							if matchedApp != nil {
								appCCacheDir = matchedApp.CCacheDir
								appStateDir = matchedApp.StateDir
								appRunLinter = matchedApp.RunLinter
								appBuilderArgs = matchedApp.BuilderArgs
								if matchedApp.Linter != nil {
									if matchedApp.Linter.Strict != nil {
										appLinterStrict = *matchedApp.Linter.Strict
									}
									appLinterIgnoreRules = matchedApp.Linter.IgnoreRules
									appLinterExceptions = matchedApp.Linter.Exceptions
									appLinterExceptionsFile = matchedApp.Linter.ExceptionsFile
								}
								if matchedApp.CCache != nil && !*matchedApp.CCache {
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
									appLinterExceptions = cfg.Linter.Exceptions
									appLinterExceptionsFile = cfg.Linter.ExceptionsFile
								}
							}

							if ccacheDirChanged {
								appCCacheDir = relCCacheDir
							}
							if stateDirChanged {
								appStateDir = relStateDir
							}
							if runLinterChanged {
								appRunLinter = relRunLinter
							}
							if builderArgChanged {
								appBuilderArgs = relBuilderArgs
							}

							appLinterExceptions, appLinterExceptionsFile = resolveLinterExceptions(
								linterExceptionsFileChanged,
								linterExceptionChanged,
								appLinterExceptions,
								appLinterExceptionsFile,
								relLinterExceptions,
								relLinterExceptionsFile,
							)

							bOpts := builder.BuildOptions{
								AppID:                row.AppID,
								Manifest:             row.Manifest,
								Arch:                 row.Arch,
								Branch:               row.Branch,
								CCacheDir:            appCCacheDir,
								StateDir:             appStateDir,
								RepoPath:             repoPath,
								RunLinter:            appRunLinter,
								LinterStrict:         appLinterStrict,
								LinterIgnoreRules:    appLinterIgnoreRules,
								LinterExceptions:     appLinterExceptions,
								LinterExceptionsFile: appLinterExceptionsFile,
								BuilderArgs:          appBuilderArgs,
							}
							if err := builder.Build(bOpts); err != nil {
								return fmt.Errorf("build failed for %s (%s): %w", row.AppID, row.Arch, err)
							}
						} else {
							iOpts := importer.ImportOptions{
								AppID:        row.AppID,
								Arch:         row.Arch,
								Branch:       row.Branch,
								BundleURL:    row.BundleURL,
								BundleSHA256: row.BundleSHA256,
								RepoPath:     repoPath,
							}
							if err := importer.Import(iOpts); err != nil {
								return fmt.Errorf("import failed for %s (%s): %w", row.AppID, row.Arch, err)
							}
						}

						// 2. Push to Registry
						pOpts := oci.PushOptions{
							AppID:         row.AppID,
							Arch:          row.Arch,
							Branch:        row.Branch,
							Registry:      cfg.Registry,
							OCIRepository: cfg.OCIRepository,
							RepoPath:      repoPath,
							RecordsDir:    recordsDir,
							GPGKeys:       keys,
							GPGPassphrase: passphrase,
							Insecure:      relInsecure,
							OCIUsername:   viper.GetString("oci_username"),
							OCIPassword:   viper.GetString("oci_password"),
							NoSign:        noSign,
							AllowUnsigned: allowUnsigned,
						}
						if _, err := oci.Push(pOpts); err != nil {
							return fmt.Errorf("push-oci failed for %s (%s): %w", row.AppID, row.Arch, err)
						}
					}
					return nil
				})
			}

			if err := g.Wait(); err != nil {
				return NewCmdErrorf(1, "Concurrency execution failed: %w", err)
			}
			logger.Info("All application publications finished successfully.")
		}

		logger.Info("Phase 3: Aggregating flatpak index references...")
		var brandLogo, brandFavicon, brandAccent, brandFooter, brandTemplate string
		if cfg != nil && cfg.Branding != nil {
			brandLogo = cfg.Branding.LogoURL
			brandFavicon = cfg.Branding.FaviconURL
			brandAccent = cfg.Branding.AccentColor
			brandFooter = cfg.Branding.FooterText
			brandTemplate = cfg.Branding.IndexTemplate
		}

		if relIndexTemplate == "" {
			relIndexTemplate = brandTemplate
		}

		pagesURL := viper.GetString("pages_url")
		if pagesURL == "" && cfg != nil {
			pagesURL = cfg.PagesURL
		}

		remoteName := relRemoteName
		if remoteName == "" {
			remoteName = cfg.RemoteName
		}
		runtimeRepo := relRuntimeRepo
		if runtimeRepo == "" {
			runtimeRepo = cfg.RuntimeRepo
		}
		repoTitle := relRepoTitle
		if repoTitle == "" {
			repoTitle = cfg.RepoTitle
		}
		repoHomepage := relRepoHomepage
		if repoHomepage == "" {
			repoHomepage = cfg.RepoHomepage
		}

		sOpts := site.SiteOptions{
			PagesURL:      pagesURL,
			RecordsDir:    recordsDir,
			SiteDir:       siteDir,
			Reconcile:     relReconcile,
			GPGKeys:       keys,
			GPGPassphrase: passphrase,
			RemoteName:    remoteName,
			RuntimeRepo:   runtimeRepo,
			RepoTitle:     repoTitle,
			RepoHomepage:  repoHomepage,
			LandingPage:   relLandingPage,
			Insecure:      relInsecure,
			LogoURL:       brandLogo,
			FaviconURL:    brandFavicon,
			AccentColor:   brandAccent,
			FooterText:    brandFooter,
			IndexTemplate: relIndexTemplate,
			NoSign:        noSign,
			AllowUnsigned: allowUnsigned,
		}

		if err := site.BuildSite(sOpts); err != nil {
			return NewCmdErrorf(1, "Site index compilation failed: %w", err)
		}

		if err := ciout.Emit(relOutputFile, []ciout.KV{
			{Key: "site-dir", Value: siteDir},
			{Key: "records-dir", Value: recordsDir},
		}); err != nil {
			return NewCmdErrorf(1, "Output emission failed: %w", err)
		}

		logger.Info("AetherPak Release completed successfully!")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(releaseCmd)

	releaseCmd.Flags().StringVar(&relBaseSHA, "base-sha", "", "git base commit SHA to diff against")
	releaseCmd.Flags().StringVar(&relForce, "force", "", "force selection ('all' or specific app ID)")
	releaseCmd.Flags().StringVar(&relWorkflowPath, "workflow-path", "", "caller workflow file path (forces rebuild if changed)")
	releaseCmd.Flags().StringSliceVar(&relGPGKeys, "gpg-key", nil, "GPG private key block(s) or path(s) to private key file(s)")
	releaseCmd.Flags().StringVar(&relCCacheDir, "ccache-dir", ".ccache", "ccache directory")
	releaseCmd.Flags().StringVar(&relStateDir, "state-dir", ".state", "builder state directory")
	releaseCmd.Flags().StringVar(&relRecordsDir, "records-dir", "records", "directory to write parallel records")
	releaseCmd.Flags().StringVar(&relSiteDir, "site-dir", "_site", "destination directory for static site assets")
	releaseCmd.Flags().IntVar(&relWorkers, "workers", 4, "number of concurrent worker threads")
	releaseCmd.Flags().BoolVar(&relRunLinter, "run-linter", false, "run flatpak-builder-lint on manifests and repositories")
	releaseCmd.Flags().StringVar(&relGPGPassphrase, "gpg-key-passphrase", "", "passphrase unlocking the GPG private key(s)")
	releaseCmd.Flags().BoolVar(&relInsecure, "insecure", false, "allow connection to insecure OCI registry (HTTP)")
	releaseCmd.Flags().StringVar(&relRepoPath, "repo-path", "repo", "path to local OSTree repository")
	releaseCmd.Flags().StringVar(&relOutputFile, "output-file", "", "write resolved outputs as dotenv KEY=VALUE (- or empty = stdout)")
	releaseCmd.Flags().StringVar(&relIndexTemplate, "index-template", "", "path to custom HTML repository index template")
	releaseCmd.Flags().BoolVar(&relNoSign, "no-sign", false, "disable GPG signing of repositories/images")
	releaseCmd.Flags().BoolVar(&relAllowUnsigned, "allow-unsigned", false, "allow publishing unsigned repository/images")

	releaseCmd.Flags().StringVar(&relRemoteName, "remote-name", "", "flatpak remote name for generated references")
	releaseCmd.Flags().StringVar(&relRuntimeRepo, "runtime-repo", "", "URL for the runtime repository (.flatpakrepo)")
	releaseCmd.Flags().StringVar(&relRepoTitle, "repo-title", "", "title for the generated .flatpakrepo file")
	releaseCmd.Flags().StringVar(&relRepoHomepage, "repo-homepage", "", "homepage URL for the generated .flatpakrepo file")
	releaseCmd.Flags().StringSliceVar(&relBuilderArgs, "builder-arg", nil, "extra argument passed through to flatpak-builder")
	releaseCmd.Flags().BoolVar(&relReconcile, "reconcile", true, "verify OCI image tags and prune missing index listings")
	releaseCmd.Flags().BoolVar(&relLandingPage, "landing-page", true, "generate an index.html landing page")
	releaseCmd.Flags().StringVar(&relLinterExceptionsFile, "linter-exceptions-file", "", "path to linter exceptions file (JSON)")
	releaseCmd.Flags().StringSliceVar(&relLinterExceptions, "linter-exception", nil, "linter exceptions to ignore")
}
