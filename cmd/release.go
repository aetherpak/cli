package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

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

		// Load signing GPG keys
		var keys []string
		for _, keyVal := range relGPGKeys {
			if keyVal != "" {
				if _, err := os.Stat(keyVal); err == nil {
					data, err := os.ReadFile(keyVal)
					if err == nil {
						keyVal = string(data)
					}
				}
				keys = append(keys, keyVal)
			}
		}
		if len(keys) == 0 {
			envKey := os.Getenv("AETHERPAK_GPG_KEY")
			if envKey != "" {
				keys = append(keys, envKey)
			}
		}

		passphraseStr := relGPGPassphrase
		if passphraseStr == "" {
			passphraseStr = os.Getenv("AETHERPAK_GPG_PASSPHRASE")
		}
		var passphrase []byte
		if passphraseStr != "" {
			passphrase = []byte(passphraseStr)
		}
		defer func() {
			if len(passphrase) > 0 {
				for i := range passphrase {
					passphrase[i] = 0
				}
			}
		}()

		noSign := relNoSign
		if !noSign {
			envVal := strings.ToLower(os.Getenv("AETHERPAK_NO_SIGN"))
			if envVal == "true" || envVal == "1" || envVal == "yes" {
				noSign = true
			}
		}
		if !noSign && cfg != nil {
			noSign = cfg.NoSign
		}

		allowUnsigned := relAllowUnsigned
		if !allowUnsigned {
			envVal := strings.ToLower(os.Getenv("AETHERPAK_ALLOW_UNSIGNED"))
			if envVal == "true" || envVal == "1" || envVal == "yes" {
				allowUnsigned = true
			}
		}

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
								}
							}

							if cmd.Flags().Changed("ccache-dir") {
								appCCacheDir = relCCacheDir
							}
							if cmd.Flags().Changed("state-dir") {
								appStateDir = relStateDir
							}
							if cmd.Flags().Changed("run-linter") {
								appRunLinter = relRunLinter
							}

							bOpts := builder.BuildOptions{
								AppID:             row.AppID,
								Manifest:          row.Manifest,
								Arch:              row.Arch,
								Branch:            row.Branch,
								CCacheDir:         appCCacheDir,
								StateDir:          appStateDir,
								RunLinter:         appRunLinter,
								LinterStrict:      appLinterStrict,
								LinterIgnoreRules: appLinterIgnoreRules,
								BuilderArgs:       appBuilderArgs,
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
							OCIRepository: cfg.RemoteName,
							RepoPath:      relRepoPath,
							RecordsDir:    relRecordsDir,
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
			relIndexTemplate = os.Getenv("AETHERPAK_INDEX_TEMPLATE")
		}
		if relIndexTemplate == "" {
			relIndexTemplate = brandTemplate
		}

		pagesURL := os.Getenv("AETHERPAK_PAGES_URL")
		if pagesURL == "" && cfg != nil {
			pagesURL = cfg.PagesURL
		}

		sOpts := site.SiteOptions{
			PagesURL:      pagesURL,
			RecordsDir:    relRecordsDir,
			SiteDir:       relSiteDir,
			Reconcile:     true,
			GPGKeys:       keys,
			GPGPassphrase: passphrase,
			RemoteName:    cfg.RemoteName,
			RuntimeRepo:   cfg.RuntimeRepo,
			RepoTitle:     cfg.RepoTitle,
			RepoHomepage:  cfg.RepoHomepage,
			LandingPage:   true,
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
			{Key: "site-dir", Value: relSiteDir},
			{Key: "records-dir", Value: relRecordsDir},
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
}
