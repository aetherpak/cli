package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/adder"
	"github.com/aetherpak/aetherpak/pkg/builder"
	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/importer"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/manifest"
	"github.com/aetherpak/aetherpak/pkg/oci"
	"github.com/aetherpak/aetherpak/pkg/repoinfo"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	pubAppID                string
	pubArch                 string
	pubBranch               string
	pubRegistry             string
	pubOCIRepo              string
	pubGPGKeys              []string
	pubGPGPassphrase        string
	pubInsecure             bool
	pubRepoPath             string
	pubCCacheDir            string
	pubStateDir             string
	pubRecordsDir           string
	pubRunLinter            bool
	pubOutputFile           string
	pubNoSign               bool
	pubAllowUnsigned        bool
	pubManifest             string
	pubBundle               string
	pubBundleURL            string
	pubBundlePath           string
	pubConfirm              bool
	pubLinterExceptionsFile string
)

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Builds/imports and pushes a single app to OCI",
	Long:  `Porcelain command that automatically executes the local build/import process and pushes the resulting application directly to the OCI registry.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		hasConfig := true
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}
		if viper.ConfigFileUsed() == "" {
			hasConfig = false
		}

		if err := config.ValidateArch(pubArch); err != nil {
			return NewCmdError(2, err)
		}

		if pubRegistry == "" {
			pubRegistry = cfg.Registry
		}
		if pubOCIRepo == "" {
			pubOCIRepo = cfg.OCIRepository
		}

		repoPath := pubRepoPath
		if !cmd.Flags().Changed("repo-path") && cfg.OutputDir != "" {
			repoPath = filepath.Join(cfg.OutputDir, "repo")
		} else if repoPath == "" {
			repoPath = "repo"
		}

		recordsDir := pubRecordsDir
		if !cmd.Flags().Changed("records-dir") && cfg.OutputDir != "" {
			recordsDir = filepath.Join(cfg.OutputDir, "records")
		} else if recordsDir == "" {
			recordsDir = "records"
		}

		// Validate mutual exclusion
		sourceCount := 0
		if pubManifest != "" {
			sourceCount++
		}
		if pubBundle != "" {
			sourceCount++
		}
		if pubBundleURL != "" {
			sourceCount++
		}
		if pubBundlePath != "" {
			sourceCount++
		}
		if sourceCount > 1 {
			return NewCmdErrorf(2, "only one of --manifest, --bundle, --bundle-url, or --bundle-path may be specified")
		}

		// Handle one-off publishes (manifest or bundle)
		if pubManifest != "" || pubBundle != "" || pubBundleURL != "" || pubBundlePath != "" {
			if pubRegistry == "" || pubOCIRepo == "" {
				return NewCmdErrorf(2, "OCI registry and repository must be specified via flags or configuration")
			}

			var resolvedAppID, resolvedArch, resolvedBranch string

			if pubManifest != "" {
				// Parse manifest
				manifestData, err := manifest.ParseManifest(pubManifest)
				if err != nil {
					return NewCmdErrorf(2, "Manifest parsing error: %w", err)
				}
				resolvedAppID = manifestData.ID
				resolvedArch = pubArch
				resolvedBranch = pubBranch
				if resolvedBranch == "" {
					if ch := resolveChannelFromEnv(); ch != "" {
						resolvedBranch = ch
					} else {
						resolvedBranch = "stable"
					}
				}

				appLinterExceptionsFile := ""
				if envVal := os.Getenv("AETHERPAK_LINTER_EXCEPTIONS"); envVal != "" {
					appLinterExceptionsFile = envVal
				}
				if cmd.Flags().Changed("linter-exceptions-file") {
					appLinterExceptionsFile = pubLinterExceptionsFile
				}

				// Run build
				buildOpts := builder.BuildOptions{
					AppID:                resolvedAppID,
					Manifest:             pubManifest,
					Arch:                 resolvedArch,
					Branch:               resolvedBranch,
					CCacheDir:            pubCCacheDir,
					StateDir:             pubStateDir,
					RepoPath:             repoPath,
					RunLinter:            pubRunLinter,
					LinterStrict:         true,
					LinterExceptionsFile: appLinterExceptionsFile,
				}
				logger.Info("Step 1: Building manifest application %s...", resolvedAppID)
				if err := builder.Build(buildOpts); err != nil {
					return NewCmdError(1, err)
				}
			} else {
				// pubBundle != "" || pubBundleURL != "" || pubBundlePath != ""
				var tempRepoDir string
				var useTempRepo bool

				// If all details provided, skip auto-detection
				if pubAppID != "" && cmd.Flags().Changed("arch") && pubBranch != "" {
					resolvedAppID = pubAppID
					resolvedArch = pubArch
					resolvedBranch = pubBranch
					useTempRepo = false
				} else {
					useTempRepo = true
					var err error
					tempRepoDir, err = os.MkdirTemp("", "aetherpak-publish-import-*")
					if err != nil {
						return NewCmdErrorf(1, "failed to create temp repo directory: %w", err)
					}
					defer os.RemoveAll(tempRepoDir)
				}

				var bundleURL, bundlePath string
				if pubBundleURL != "" {
					bundleURL = pubBundleURL
				}
				if pubBundlePath != "" {
					bundlePath = pubBundlePath
				}
				if pubBundle != "" {
					if strings.HasPrefix(pubBundle, "http://") || strings.HasPrefix(pubBundle, "https://") {
						bundleURL = pubBundle
					} else {
						bundlePath = pubBundle
					}
				}

				destRepo := repoPath
				if destRepo == "" {
					destRepo = "repo"
				}

				importRepo := destRepo
				if useTempRepo {
					importRepo = tempRepoDir
				}

				// For import we need empty values for properties we want importer to auto-detect
				importAppID := pubAppID
				importArch := pubArch
				if !cmd.Flags().Changed("arch") {
					importArch = ""
				}
				importBranch := pubBranch

				importOpts := importer.ImportOptions{
					AppID:      importAppID,
					Arch:       importArch,
					Branch:     importBranch,
					BundleURL:  bundleURL,
					BundlePath: bundlePath,
					RepoPath:   importRepo,
				}

				bundleDisplay := pubBundle
				if bundleDisplay == "" {
					if pubBundleURL != "" {
						bundleDisplay = pubBundleURL
					} else {
						bundleDisplay = pubBundlePath
					}
				}
				logger.Info("Step 1: Importing bundle package %s...", bundleDisplay)
				if err := importer.Import(importOpts); err != nil {
					return NewCmdError(1, err)
				}

				if useTempRepo {
					// Resolve auto-detected coordinates
					info, err := repoinfo.Resolve(tempRepoDir)
					if err != nil {
						return NewCmdErrorf(1, "failed to resolve imported bundle ref: %w", err)
					}

					resolvedAppID = pubAppID
					if resolvedAppID == "" {
						resolvedAppID = info.AppID
					}
					resolvedArch = pubArch
					if !cmd.Flags().Changed("arch") {
						resolvedArch = info.Arch
					}
					resolvedBranch = pubBranch
					if resolvedBranch == "" {
						resolvedBranch = info.Branch
					}
				}

				// Prompt for confirmation if interactive and not bypassed
				if isInteractive() && !pubConfirm {
					var confirm bool
					err := huh.NewConfirm().
						Title(fmt.Sprintf("Do you want to publish %s (%s, channel: %s)?", resolvedAppID, resolvedArch, resolvedBranch)).
						Value(&confirm).
						Run()
					if err != nil {
						return err
					}
					if !confirm {
						return fmt.Errorf("publish cancelled by user")
					}
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

					destRef := fmt.Sprintf("app/%s/%s/%s", resolvedAppID, resolvedArch, resolvedBranch)
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

			// Push to registry
			return pushAndEmit(resolvedAppID, resolvedArch, resolvedBranch, pubRegistry, pubOCIRepo, repoPath, recordsDir)
		}

		// Otherwise, publish config-driven apps
		var appsToPublish []*config.App
		if pubAppID != "" {
			var targetApp *config.App
			for i := range cfg.Apps {
				if cfg.Apps[i].ID == pubAppID {
					targetApp = &cfg.Apps[i]
					break
				}
			}
			if targetApp == nil {
				return NewCmdErrorf(1, "app %q not found in config", pubAppID)
			}
			appsToPublish = append(appsToPublish, targetApp)
		} else {
			if !hasConfig {
				return NewCmdError(2, fmt.Errorf("no application ID provided and no configuration file found"))
			}
			if len(cfg.Apps) == 0 {
				return NewCmdError(2, fmt.Errorf("no applications found in configuration file"))
			}
			for i := range cfg.Apps {
				appsToPublish = append(appsToPublish, &cfg.Apps[i])
			}
		}

		for _, targetApp := range appsToPublish {
			appBranch := pubBranch
			if appBranch == "" {
				appBranch = targetApp.Branch
			}
			if appBranch == "" {
				if ch := resolveChannelFromEnv(); ch != "" {
					appBranch = ch
				} else {
					appBranch = "stable"
				}
			}

			appRegistry := pubRegistry
			if appRegistry == "" {
				appRegistry = cfg.Registry
			}
			appOCIRepo := pubOCIRepo
			if appOCIRepo == "" {
				appOCIRepo = cfg.OCIRepository
			}

			// Phase 1: Local compilation or import
			if targetApp.Manifest != "" {
				// Resolve build option defaults from configuration
				var appCCacheDir = ".ccache"
				var appStateDir = ".state"
				var appRunLinter = false
				var appLinterStrict = true
				var appLinterIgnoreRules []string
				var appLinterExceptionsFile = ""

				if targetApp != nil {
					appCCacheDir = targetApp.CCacheDir
					appStateDir = targetApp.StateDir
					appRunLinter = targetApp.RunLinter
					if targetApp.Linter != nil {
						if targetApp.Linter.Strict != nil {
							appLinterStrict = *targetApp.Linter.Strict
						}
						appLinterIgnoreRules = targetApp.Linter.IgnoreRules
						appLinterExceptionsFile = targetApp.Linter.ExceptionsFile
					}
					if targetApp.CCache != nil && !*targetApp.CCache {
						appCCacheDir = ""
					}
				}

				// Apply CLI flag overrides if explicitly passed
				if cmd.Flags().Changed("ccache-dir") {
					appCCacheDir = pubCCacheDir
				}
				if cmd.Flags().Changed("state-dir") {
					appStateDir = pubStateDir
				}
				if cmd.Flags().Changed("run-linter") {
					appRunLinter = pubRunLinter
				}

				if envVal := os.Getenv("AETHERPAK_LINTER_EXCEPTIONS"); envVal != "" {
					appLinterExceptionsFile = envVal
				}
				if cmd.Flags().Changed("linter-exceptions-file") {
					appLinterExceptionsFile = pubLinterExceptionsFile
				}

				opts := builder.BuildOptions{
					AppID:                targetApp.ID,
					Manifest:             targetApp.Manifest,
					Arch:                 pubArch,
					Branch:               appBranch,
					CCacheDir:            appCCacheDir,
					StateDir:             appStateDir,
					RepoPath:             repoPath,
					RunLinter:            appRunLinter,
					LinterStrict:         appLinterStrict,
					LinterIgnoreRules:    appLinterIgnoreRules,
					LinterExceptionsFile: appLinterExceptionsFile,
					BuilderArgs:          targetApp.BuilderArgs,
				}
				logger.Info("Step 1: Building manifest application %s...", targetApp.ID)
				if err := builder.Build(opts); err != nil {
					return NewCmdError(1, err)
				}
			} else {
				bundle, exists := targetApp.Bundles[pubArch]
				if !exists {
					return NewCmdErrorf(1, "no bundle configured for architecture %q for app %s", pubArch, targetApp.ID)
				}

				opts := importer.ImportOptions{
					AppID:        targetApp.ID,
					Arch:         pubArch,
					Branch:       appBranch,
					BundleURL:    bundle.URL,
					BundleSHA256: bundle.SHA256,
					RepoPath:     repoPath,
				}
				logger.Info("Step 1: Importing bundle package %s...", targetApp.ID)
				if err := importer.Import(opts); err != nil {
					return NewCmdError(1, err)
				}
			}

			if err := pushAndEmit(targetApp.ID, pubArch, appBranch, appRegistry, appOCIRepo, repoPath, recordsDir); err != nil {
				return err
			}
		}

		return nil
	},
}

func pushAndEmit(appID, arch, branch, registry, ociRepo, repoPath, recordsDir string) error {
	// Load GPG keys from files if passed (keys will already contain GPG keys from flag or env var)
	var keys []string
	for _, keyVal := range pubGPGKeys {
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

	var passphrase []byte
	if pubGPGPassphrase != "" {
		passphrase = []byte(pubGPGPassphrase)
	}

	logger.Info("Step 2: Pushing %s to registry...", appID)
	pushOpts := oci.PushOptions{
		AppID:         appID,
		Arch:          arch,
		Branch:        branch,
		Registry:      registry,
		OCIRepository: ociRepo,
		RepoPath:      repoPath,
		RecordsDir:    recordsDir,
		GPGKeys:       keys,
		GPGPassphrase: passphrase,
		Insecure:      pubInsecure,
		OCIUsername:   viper.GetString("oci_username"),
		OCIPassword:   viper.GetString("oci_password"),
		NoSign:        pubNoSign,
		AllowUnsigned: pubAllowUnsigned,
	}

	res, err := oci.Push(pushOpts)
	if len(passphrase) > 0 {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}
	if err != nil {
		return NewCmdError(1, err)
	}

	if err := ciout.Emit(pubOutputFile, []ciout.KV{
		{Key: "app-id", Value: appID},
		{Key: "arch", Value: arch},
		{Key: "branch", Value: branch},
		{Key: "cell-dir", Value: res.CellDir},
		{Key: "digest", Value: res.Digest},
		{Key: "tag", Value: res.Tag},
	}); err != nil {
		return NewCmdError(1, err)
	}

	logger.SuccessBanner("Publish Completed", fmt.Sprintf("Successfully built and published %s (%s) to %s/%s.", appID, arch, registry, ociRepo))
	return nil
}

func init() {
	RootCmd.AddCommand(publishCmd)

	publishCmd.Flags().StringVar(&pubAppID, "app-id", "", "app ID (reverse-DNS format)")
	publishCmd.Flags().StringVar(&pubAppID, "app", "", "deprecated alias for --app-id")
	_ = publishCmd.Flags().MarkDeprecated("app", "please use --app-id instead")
	publishCmd.Flags().StringVar(&pubArch, "arch", adder.DefaultArch(), "target CPU architecture")
	publishCmd.Flags().StringVar(&pubBranch, "branch", "", "published branch channel")
	publishCmd.Flags().StringVar(&pubRegistry, "registry", "", "target OCI registry host")
	publishCmd.Flags().StringVar(&pubOCIRepo, "oci-repository", "", "target repository path/name")
	publishCmd.Flags().StringSliceVar(&pubGPGKeys, "gpg-key", nil, "GPG private key block(s) or path(s) to private key file(s)")
	publishCmd.Flags().StringVar(&pubCCacheDir, "ccache-dir", ".ccache", "ccache directory")
	publishCmd.Flags().StringVar(&pubStateDir, "state-dir", ".state", "builder state directory")
	publishCmd.Flags().StringVar(&pubRecordsDir, "records-dir", "records", "directory to write parallel records")
	publishCmd.Flags().BoolVar(&pubRunLinter, "run-linter", false, "run flatpak-builder-lint before and after build")
	publishCmd.Flags().StringVar(&pubGPGPassphrase, "gpg-key-passphrase", "", "passphrase unlocking the GPG private key(s)")
	publishCmd.Flags().BoolVar(&pubInsecure, "insecure", false, "allow connection to insecure OCI registry (HTTP)")
	publishCmd.Flags().StringVar(&pubRepoPath, "repo-path", "repo", "path to local OSTree repository")
	publishCmd.Flags().StringVar(&pubOutputFile, "output-file", "", "write resolved outputs as dotenv KEY=VALUE (- or empty = stdout)")
	publishCmd.Flags().BoolVar(&pubNoSign, "no-sign", false, "disable GPG signing of repositories/images")
	publishCmd.Flags().BoolVar(&pubAllowUnsigned, "allow-unsigned", false, "allow publishing unsigned repository/images")
	publishCmd.Flags().StringVar(&pubManifest, "manifest", "", "path to a local Flatpak manifest file (bypasses config)")
	publishCmd.Flags().StringVar(&pubBundle, "bundle", "", "Flatpak bundle URL or path to import and publish")
	publishCmd.Flags().StringVar(&pubBundleURL, "bundle-url", "", "Flatpak bundle URL to import and publish")
	publishCmd.Flags().StringVar(&pubBundlePath, "bundle-path", "", "Flatpak bundle local path to import and publish")
	publishCmd.Flags().BoolVar(&pubConfirm, "confirm", false, "skip interactive confirmation prompt")
	publishCmd.Flags().StringVar(&pubLinterExceptionsFile, "linter-exceptions-file", "", "path to linter exceptions file (JSON)")
}
