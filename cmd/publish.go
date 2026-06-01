package cmd

import (
	"fmt"
	"os"

	"github.com/aetherpak/aetherpak/pkg/builder"
	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/importer"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/oci"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	pubAppID         string
	pubArch          string
	pubBranch        string
	pubRegistry      string
	pubOCIRepo       string
	pubGPGKeys       []string
	pubGPGPassphrase string
	pubInsecure      bool
	pubRepoPath      string
	pubCCacheDir     string
	pubStateDir      string
	pubRecordsDir    string
	pubRunLinter     bool
	pubOutputFile    string
	pubNoSign        bool
	pubAllowUnsigned bool
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

				if targetApp != nil {
					appCCacheDir = targetApp.CCacheDir
					appStateDir = targetApp.StateDir
					appRunLinter = targetApp.RunLinter
					if targetApp.Linter != nil {
						if targetApp.Linter.Strict != nil {
							appLinterStrict = *targetApp.Linter.Strict
						}
						appLinterIgnoreRules = targetApp.Linter.IgnoreRules
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

				opts := builder.BuildOptions{
					AppID:             targetApp.ID,
					Manifest:          targetApp.Manifest,
					Arch:              pubArch,
					Branch:            appBranch,
					CCacheDir:         appCCacheDir,
					StateDir:          appStateDir,
					RunLinter:         appRunLinter,
					LinterStrict:      appLinterStrict,
					LinterIgnoreRules: appLinterIgnoreRules,
					BuilderArgs:       targetApp.BuilderArgs,
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
				}
				logger.Info("Step 1: Importing bundle package %s...", targetApp.ID)
				if err := importer.Import(opts); err != nil {
					return NewCmdError(1, err)
				}
			}

			// Phase 2: OCI registry push
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

			noSign := pubNoSign
			allowUnsigned := pubAllowUnsigned

			logger.Info("Step 2: Pushing %s to registry...", targetApp.ID)
			pushOpts := oci.PushOptions{
				AppID:         targetApp.ID,
				Arch:          pubArch,
				Branch:        appBranch,
				Registry:      appRegistry,
				OCIRepository: appOCIRepo,
				RepoPath:      pubRepoPath,
				RecordsDir:    pubRecordsDir,
				GPGKeys:       keys,
				GPGPassphrase: passphrase,
				Insecure:      pubInsecure,
				OCIUsername:   viper.GetString("oci_username"),
				OCIPassword:   viper.GetString("oci_password"),
				NoSign:        noSign,
				AllowUnsigned: allowUnsigned,
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
				{Key: "app-id", Value: targetApp.ID},
				{Key: "arch", Value: pubArch},
				{Key: "branch", Value: appBranch},
				{Key: "cell-dir", Value: res.CellDir},
				{Key: "digest", Value: res.Digest},
				{Key: "tag", Value: res.Tag},
			}); err != nil {
				return NewCmdError(1, err)
			}

			logger.SuccessBanner("Publish Completed", fmt.Sprintf("Successfully built and published %s (%s) to %s/%s.", targetApp.ID, pubArch, appRegistry, appOCIRepo))
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(publishCmd)

	publishCmd.Flags().StringVar(&pubAppID, "app-id", "", "app ID (reverse-DNS format)")
	publishCmd.Flags().StringVar(&pubAppID, "app", "", "deprecated alias for --app-id")
	_ = publishCmd.Flags().MarkDeprecated("app", "please use --app-id instead")
	publishCmd.Flags().StringVar(&pubArch, "arch", "x86_64", "target CPU architecture")
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
}
