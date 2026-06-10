package cmd

import (
	"fmt"
	"os"
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
	"github.com/aetherpak/aetherpak/pkg/scm"
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
	pubBundles              []string
	pubBundleURLs           []string
	pubBundlePaths          []string
	pubConfirm              bool
	pubLinterExceptionsFile string
	pubLinterExceptions     []string
	pubDryRun               bool
)

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Builds/imports and pushes a single app to OCI",
	Long:  `Porcelain command that automatically executes the local build/import process and pushes the resulting application directly to the OCI registry.`,
	RunE:  runPublish,
}

func runPublish(cmd *cobra.Command, args []string) error {
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

	var pubForceBranch string
	if pubAppID != "" {
		cleanID, br := parseAppIDRef(pubAppID)
		pubAppID = cleanID
		pubForceBranch = br
	}

	if pubBranch == "" {
		pubBranch = pubForceBranch
	}

	if pubRegistry == "" {
		pubRegistry = cfg.Registry
	}
	if pubRegistry == "" {
		pubRegistry = scm.Registry()
	}
	if pubOCIRepo == "" {
		pubOCIRepo = cfg.OCIRepository
	}
	if pubOCIRepo == "" {
		pubOCIRepo = scm.OCIRepository()
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

	pubBundles = SplitAndCleanSlice(pubBundles)
	pubBundleURLs = SplitAndCleanSlice(pubBundleURLs)
	pubBundlePaths = SplitAndCleanSlice(pubBundlePaths)

	// Validate mutual exclusion: manifest cannot be specified with any bundle input
	if pubManifest != "" && (len(pubBundles) > 0 || len(pubBundleURLs) > 0 || len(pubBundlePaths) > 0) {
		return NewCmdErrorf(2, "cannot specify both --manifest and bundle inputs (--bundle, --bundle-url, or --bundle-path)")
	}

	// Handle one-off publishes (manifest or bundle)
	if pubManifest != "" || len(pubBundles) > 0 || len(pubBundleURLs) > 0 || len(pubBundlePaths) > 0 {
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

			appLinterExceptions, appLinterExceptionsFile := resolveLinterExceptions(
				cmd.Flags().Changed("linter-exceptions-file"),
				cmd.Flags().Changed("linter-exception"),
				nil,
				"",
				pubLinterExceptions,
				pubLinterExceptionsFile,
			)

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
				LinterExceptions:     appLinterExceptions,
				LinterExceptionsFile: appLinterExceptionsFile,
			}
			logger.Info("Step 1: Building manifest application %s...", resolvedAppID)
			if err := builder.Build(buildOpts); err != nil {
				return NewCmdError(1, err)
			}

			// Push to registry
			refs, err := repoinfo.ResolveAll(nil, repoPath)
			if err != nil {
				return NewCmdErrorf(1, "failed to resolve refs for publishing: %w", err)
			}

			var extensionIDs []string
			if m, err := manifest.ParseManifest(pubManifest); err == nil {
				extensionIDs = m.ExtensionIDs
			}

			var pushedAny bool
			for _, ref := range refs {
				if manifest.IsRefRelated(ref.AppID, resolvedAppID, extensionIDs) {
					if err := pushAndEmit(ref.AppID, ref.Arch, ref.Branch, pubRegistry, pubOCIRepo, repoPath, recordsDir); err != nil {
						return err
					}
					pushedAny = true
				}
			}
			if !pushedAny {
				return NewCmdErrorf(1, "no related refs found to push")
			}
			return nil
		} else {
			// Expand globs for local bundle paths
			var resolvedPaths []string
			for _, pat := range pubBundlePaths {
				matches, err := filepath.Glob(pat)
				if err != nil {
					matches = []string{pat}
				}
				if len(matches) == 0 {
					matches = []string{pat}
				}
				resolvedPaths = append(resolvedPaths, matches...)
			}
			// Also handle --bundle, which could be a local path with glob or a URL.
			var resolvedBundles []string
			for _, pat := range pubBundles {
				if strings.HasPrefix(pat, "http://") || strings.HasPrefix(pat, "https://") {
					resolvedBundles = append(resolvedBundles, pat)
				} else {
					matches, err := filepath.Glob(pat)
					if err != nil {
						matches = []string{pat}
					}
					if len(matches) == 0 {
						matches = []string{pat}
					}
					resolvedBundles = append(resolvedBundles, matches...)
				}
			}

			totalBundles := len(resolvedBundles) + len(pubBundleURLs) + len(resolvedPaths)
			if totalBundles > 1 {
				if pubAppID != "" {
					return NewCmdError(2, fmt.Errorf("cannot specify --app-id when publishing multiple bundles; coordinates must be auto-detected from each bundle's internal metadata"))
				}
				if cmd.Flags().Changed("arch") {
					return NewCmdError(2, fmt.Errorf("cannot specify --arch when publishing multiple bundles; coordinates must be auto-detected from each bundle's internal metadata"))
				}
			}

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

			type importJob struct {
				bundleURL  string
				bundlePath string
			}
			var importJobs []importJob
			for _, item := range resolvedBundles {
				if strings.HasPrefix(item, "http://") || strings.HasPrefix(item, "https://") {
					importJobs = append(importJobs, importJob{bundleURL: item})
				} else {
					importJobs = append(importJobs, importJob{bundlePath: item})
				}
			}
			for _, url := range pubBundleURLs {
				importJobs = append(importJobs, importJob{bundleURL: url})
			}
			for _, path := range resolvedPaths {
				importJobs = append(importJobs, importJob{bundlePath: path})
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

			for _, job := range importJobs {
				importOpts := importer.ImportOptions{
					AppID:      importAppID,
					Arch:       importArch,
					Branch:     importBranch,
					BundleURL:  job.bundleURL,
					BundlePath: job.bundlePath,
					RepoPath:   importRepo,
				}
				bundleDisplay := job.bundleURL
				if bundleDisplay == "" {
					bundleDisplay = job.bundlePath
				}
				logger.Info("Step 1: Importing bundle package %s...", bundleDisplay)
				if err := importer.Import(importOpts); err != nil {
					return NewCmdError(1, err)
				}
			}

			var resolvedApps []repoinfo.Info
			if useTempRepo {
				infos, err := repoinfo.ResolveAll(nil, tempRepoDir)
				if err != nil {
					return NewCmdErrorf(1, "failed to resolve imported bundle refs: %w", err)
				}
				resolvedApps = infos
			} else {
				resolvedApps = []repoinfo.Info{{
					AppID:    resolvedAppID,
					Arch:     resolvedArch,
					Branch:   resolvedBranch,
					RepoPath: destRepo,
				}}
			}

			// Prompt for confirmation if interactive and not bypassed
			if isInteractive() && !pubConfirm {
				var confirm bool
				var appDetails []string
				for _, ra := range resolvedApps {
					appDetails = append(appDetails, fmt.Sprintf("%s (%s, channel: %s)", ra.AppID, ra.Arch, ra.Branch))
				}
				err := huh.NewConfirm().
					Title(fmt.Sprintf("Do you want to publish the following applications?\n- %s", strings.Join(appDetails, "\n- "))).
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
				if err := importer.RebindRefs(importer.RebindRefsOptions{
					SrcRepo:  tempRepoDir,
					DestRepo: destRepo,
					Refs:     resolvedApps,
				}); err != nil {
					return NewCmdError(1, err)
				}
			}

			// Push to registry
			for _, ra := range resolvedApps {
				if err := pushAndEmit(ra.AppID, ra.Arch, ra.Branch, pubRegistry, pubOCIRepo, destRepo, recordsDir); err != nil {
					return err
				}
			}
			return nil
		}
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
		if appRegistry == "" {
			appRegistry = scm.Registry()
		}
		appOCIRepo := pubOCIRepo
		if appOCIRepo == "" {
			appOCIRepo = cfg.OCIRepository
		}
		if appOCIRepo == "" {
			appOCIRepo = scm.OCIRepository()
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
			var appLinterExceptions []string

			if targetApp != nil {
				appCCacheDir = targetApp.CCacheDir
				appStateDir = targetApp.StateDir
				appRunLinter = targetApp.RunLinter
				if targetApp.Linter != nil {
					if targetApp.Linter.Strict != nil {
						appLinterStrict = *targetApp.Linter.Strict
					}
					appLinterIgnoreRules = targetApp.Linter.IgnoreRules
					appLinterExceptions = targetApp.Linter.Exceptions
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

			appLinterExceptions, appLinterExceptionsFile = resolveLinterExceptions(
				cmd.Flags().Changed("linter-exceptions-file"),
				cmd.Flags().Changed("linter-exception"),
				appLinterExceptions,
				appLinterExceptionsFile,
				pubLinterExceptions,
				pubLinterExceptionsFile,
			)

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
				LinterExceptions:     appLinterExceptions,
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

		if targetApp.Manifest != "" {
			refs, err := repoinfo.ResolveAll(nil, repoPath)
			if err != nil {
				return NewCmdErrorf(1, "failed to resolve refs for publishing: %w", err)
			}

			var extensionIDs []string
			if m, err := manifest.ParseManifest(targetApp.Manifest); err == nil {
				extensionIDs = m.ExtensionIDs
			}

			var pushedAny bool
			for _, ref := range refs {
				if manifest.IsRefRelated(ref.AppID, targetApp.ID, extensionIDs) {
					if err := pushAndEmit(ref.AppID, ref.Arch, ref.Branch, appRegistry, appOCIRepo, repoPath, recordsDir); err != nil {
						return err
					}
					pushedAny = true
				}
			}
			if !pushedAny {
				return NewCmdErrorf(1, "no related refs found to push")
			}
		} else {
			if err := pushAndEmit(targetApp.ID, pubArch, appBranch, appRegistry, appOCIRepo, repoPath, recordsDir); err != nil {
				return err
			}
		}
	}

	return nil
}

func pushAndEmit(appID, arch, branch, registry, ociRepo, repoPath, recordsDir string) error {
	// Load GPG keys from files if passed (keys will already contain GPG keys from flag or env var)
	keys := pubGPGKeys

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
		DryRun:        pubDryRun,
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
	publishCmd.Flags().BoolVar(&pubDryRun, "dry-run", false, "simulate publishing without writing to remote registry or records")
	publishCmd.Flags().StringVar(&pubManifest, "manifest", "", "path to a local Flatpak manifest file (bypasses config)")
	publishCmd.Flags().StringSliceVar(&pubBundles, "bundle", nil, "Flatpak bundle URL(s) or path(s) to import and publish")
	publishCmd.Flags().StringSliceVar(&pubBundleURLs, "bundle-url", nil, "Flatpak bundle URL(s) to import and publish")
	publishCmd.Flags().StringSliceVar(&pubBundlePaths, "bundle-path", nil, "Flatpak bundle local path(s) to import and publish (supports globs)")
	publishCmd.Flags().BoolVar(&pubConfirm, "confirm", false, "skip interactive confirmation prompt")
	publishCmd.Flags().StringVar(&pubLinterExceptionsFile, "linter-exceptions-file", "", "path to linter exceptions file (JSON)")
	publishCmd.Flags().StringSliceVar(&pubLinterExceptions, "linter-exception", nil, "linter exceptions to ignore")
}
