package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/oci"
	"github.com/aetherpak/aetherpak/pkg/repoinfo"
	"github.com/aetherpak/aetherpak/pkg/scm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	pushAppID         string
	pushArch          string
	pushBranch        string
	pushRegistry      string
	pushOCIRepository string
	pushRepoPath      string
	pushRecordsDir    string
	pushGPGKeys       []string
	pushGPGPassphrase string
	pushInsecure      bool
	pushOutputFile    string
	pushNoSign        bool
	pushAllowUnsigned bool
)

var pushOCICmd = &cobra.Command{
	Use:   "push-oci",
	Short: "Converts and pushes an OSTree branch to an OCI registry",
	Long:  `Transforms local Flatpak applications built in an OSTree repo to OCI layer structures, signs the descriptors, and pushes them to GHCR.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		hasConfig := true
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}
		if viper.ConfigFileUsed() == "" {
			hasConfig = false
		}

		if pushArch == "" {
			pushArch = "x86_64"
		}
		if err := config.ValidateArch(pushArch); err != nil {
			return NewCmdError(2, err)
		}

		repoPath := pushRepoPath
		if !cmd.Flags().Changed("repo-path") && cfg.OutputDir != "" {
			repoPath = filepath.Join(cfg.OutputDir, "repo")
		} else if repoPath == "" {
			repoPath = "repo"
		}

		recordsDir := pushRecordsDir
		if !cmd.Flags().Changed("records-dir") && cfg.OutputDir != "" {
			recordsDir = filepath.Join(cfg.OutputDir, "records")
		} else if recordsDir == "" {
			recordsDir = "records"
		}

		type pushTarget struct {
			AppID   string
			Arch    string
			Branch  string
			RefType string
		}

		var targets []pushTarget

		// 1. Try to auto-detect from the repository refs first (if repoPath exists and has refs)
		if repoRefs, err := repoinfo.ResolveAll(repoPath); err == nil && len(repoRefs) > 0 {
			for _, refInfo := range repoRefs {
				if pushAppID != "" && refInfo.AppID != pushAppID {
					continue
				}
				if cmd.Flags().Changed("arch") && refInfo.Arch != pushArch {
					continue
				}
				if pushBranch != "" && refInfo.Branch != pushBranch {
					continue
				}
				targets = append(targets, pushTarget{
					AppID:   refInfo.AppID,
					Arch:    refInfo.Arch,
					Branch:  refInfo.Branch,
					RefType: refInfo.RefType,
				})
			}
			if pushAppID != "" && len(targets) == 0 {
				return NewCmdErrorf(1, "no refs found in repository matching app-id %q", pushAppID)
			}
		}

		// 2. Fall back to config/flags if no matching targets resolved from repository
		if len(targets) == 0 {
			var appsToPush []*config.App
			if pushAppID != "" {
				var targetApp *config.App
				for i := range cfg.Apps {
					if cfg.Apps[i].ID == pushAppID {
						targetApp = &cfg.Apps[i]
						break
					}
				}
				if targetApp == nil {
					if hasConfig {
						return NewCmdErrorf(1, "app %q not found in config", pushAppID)
					}
					targetApp = &config.App{
						ID: pushAppID,
					}
					targetApp.Normalize()
				}
				appsToPush = append(appsToPush, targetApp)
			} else {
				if !hasConfig {
					return NewCmdError(2, fmt.Errorf("no application ID provided and no configuration file found"))
				}
				if len(cfg.Apps) == 0 {
					return NewCmdError(2, fmt.Errorf("no applications found in configuration file"))
				}
				for i := range cfg.Apps {
					appsToPush = append(appsToPush, &cfg.Apps[i])
				}
			}

			for _, targetApp := range appsToPush {
				appBranch := pushBranch
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

				appArch := pushArch
				if appArch == "" {
					appArch = "x86_64"
				}

				targets = append(targets, pushTarget{
					AppID:  targetApp.ID,
					Arch:   appArch,
					Branch: appBranch,
				})
			}
		}

		if pushRegistry == "" {
			pushRegistry = cfg.Registry
		}
		if pushRegistry == "" {
			pushRegistry = scm.Registry()
		}
		if pushRegistry == "" {
			return NewCmdError(2, fmt.Errorf("registry is required"))
		}

		if pushOCIRepository == "" {
			pushOCIRepository = cfg.OCIRepository
		}
		if pushOCIRepository == "" {
			pushOCIRepository = scm.OCIRepository()
		}

		for _, target := range targets {
			appOCIRepository := pushOCIRepository
			if appOCIRepository == "" {
				appOCIRepository = cfg.OCIRepository
			}
			if appOCIRepository == "" {
				appOCIRepository = scm.OCIRepository()
			}

			// Read GPG keys from files if passed
			var keys []string
			for _, keyVal := range pushGPGKeys {
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
			if pushGPGPassphrase != "" {
				passphrase = []byte(pushGPGPassphrase)
			}

			noSign := pushNoSign
			allowUnsigned := pushAllowUnsigned

			opts := oci.PushOptions{
				AppID:         target.AppID,
				Arch:          target.Arch,
				Branch:        target.Branch,
				Registry:      pushRegistry,
				OCIRepository: appOCIRepository,
				RepoPath:      repoPath,
				RecordsDir:    recordsDir,
				GPGKeys:       keys,
				GPGPassphrase: passphrase,
				Insecure:      pushInsecure,
				OCIUsername:   viper.GetString("oci_username"),
				OCIPassword:   viper.GetString("oci_password"),
				NoSign:        noSign,
				AllowUnsigned: allowUnsigned,
				RefType:       target.RefType,
			}

			res, err := oci.Push(opts)
			if len(passphrase) > 0 {
				for i := range passphrase {
					passphrase[i] = 0
				}
			}
			if err != nil {
				return NewCmdError(1, err)
			}

			if err := ciout.Emit(pushOutputFile, []ciout.KV{
				{Key: "app-id", Value: target.AppID},
				{Key: "arch", Value: target.Arch},
				{Key: "branch", Value: target.Branch},
				{Key: "cell-dir", Value: res.CellDir},
				{Key: "digest", Value: res.Digest},
				{Key: "tag", Value: res.Tag},
			}); err != nil {
				return NewCmdError(1, err)
			}
			logger.SuccessBanner("Push Completed", fmt.Sprintf("Successfully exported and pushed %s (%s) to registry %s/%s.", target.AppID, target.Arch, pushRegistry, appOCIRepository))
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(pushOCICmd)

	pushOCICmd.Flags().StringVar(&pushAppID, "app-id", "", "app ID (reverse-DNS format)")
	pushOCICmd.Flags().StringVar(&pushAppID, "app", "", "deprecated alias for --app-id")
	_ = pushOCICmd.Flags().MarkDeprecated("app", "please use --app-id instead")
	pushOCICmd.Flags().StringVar(&pushArch, "arch", "x86_64", "target CPU architecture")
	pushOCICmd.Flags().StringVar(&pushBranch, "branch", "", "published branch channel")
	pushOCICmd.Flags().StringVar(&pushRegistry, "registry", "", "target OCI registry host")
	pushOCICmd.Flags().StringVar(&pushOCIRepository, "oci-repository", "", "target repository path/name")
	pushOCICmd.Flags().StringVar(&pushRepoPath, "repo-path", "repo", "path to local OSTree repository")
	pushOCICmd.Flags().StringVar(&pushRecordsDir, "records-dir", "records", "directory to write parallel records")
	pushOCICmd.Flags().StringSliceVar(&pushGPGKeys, "gpg-key", nil, "GPG private key block(s) or path(s) to private key file(s)")
	pushOCICmd.Flags().StringVar(&pushGPGPassphrase, "gpg-key-passphrase", "", "passphrase unlocking the GPG private key(s)")
	pushOCICmd.Flags().BoolVar(&pushInsecure, "insecure", false, "allow connection to insecure OCI registry (HTTP)")
	pushOCICmd.Flags().StringVar(&pushOutputFile, "output-file", "", "write resolved outputs as dotenv KEY=VALUE (- or empty = stdout)")
	pushOCICmd.Flags().BoolVar(&pushNoSign, "no-sign", false, "disable GPG signing of repositories/images")
	pushOCICmd.Flags().BoolVar(&pushAllowUnsigned, "allow-unsigned", false, "allow publishing unsigned repository/images")
}
