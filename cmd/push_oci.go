package cmd

import (
	"fmt"
	"os"

	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/oci"
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
)

var pushOCICmd = &cobra.Command{
	Use:   "push-oci",
	Short: "Converts and pushes an OSTree branch to an OCI registry",
	Long:  `Transforms local Flatpak applications built in an OSTree repo to OCI layer structures, signs the descriptors, and pushes them to GHCR.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err == nil {
			// Populate from config fallbacks if flags are missing
			if pushRegistry == "" {
				pushRegistry = cfg.Registry
			}
			if pushOCIRepository == "" && len(cfg.Apps) > 0 {
				// Default repo configuration
				pushOCIRepository = cfg.RemoteName
			}

			// App matching configuration lookup
			if pushAppID == "" && len(cfg.Apps) > 0 {
				pushAppID = cfg.Apps[0].ID
				if pushBranch == "" {
					pushBranch = cfg.Apps[0].Branch
				}
			} else {
				for _, app := range cfg.Apps {
					if app.ID == pushAppID && pushBranch == "" {
						pushBranch = app.Branch
						break
					}
				}
			}
		}

		if pushAppID == "" {
			return NewCmdError(2, fmt.Errorf("app is required"))
		}
		if pushRegistry == "" {
			return NewCmdError(2, fmt.Errorf("registry is required"))
		}

		if pushArch == "" {
			pushArch = "x86_64"
		}
		if pushBranch == "" {
			if ch := resolveChannelFromEnv(); ch != "" {
				pushBranch = ch
			} else {
				pushBranch = "stable"
			}
		}

		// Read GPG keys from files or environment variables if passed
		var keys []string
		for _, keyVal := range pushGPGKeys {
			if keyVal != "" {
				// Try reading as file path first
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
			// Fallback check on standard environment variable
			envKey := os.Getenv("AETHERPAK_GPG_KEY")
			if envKey != "" {
				keys = append(keys, envKey)
			}
		}

		passphrase := pushGPGPassphrase
		if passphrase == "" {
			passphrase = os.Getenv("AETHERPAK_GPG_PASSPHRASE")
		}

		opts := oci.PushOptions{
			AppID:         pushAppID,
			Arch:          pushArch,
			Branch:        pushBranch,
			Registry:      pushRegistry,
			OCIRepository: pushOCIRepository,
			RepoPath:      pushRepoPath,
			RecordsDir:    pushRecordsDir,
			GPGKeys:       keys,
			GPGPassphrase: passphrase,
			Insecure:      pushInsecure,
			OCIUsername:   viper.GetString("oci_username"),
			OCIPassword:   viper.GetString("oci_password"),
		}

		res, err := oci.Push(opts)
		if err != nil {
			return NewCmdError(1, err)
		}
		if err := ciout.Emit(pushOutputFile, []ciout.KV{
			{Key: "app-id", Value: pushAppID},
			{Key: "arch", Value: pushArch},
			{Key: "branch", Value: pushBranch},
			{Key: "cell-dir", Value: res.CellDir},
			{Key: "digest", Value: res.Digest},
			{Key: "tag", Value: res.Tag},
		}); err != nil {
			return NewCmdError(1, err)
		}
		logger.SuccessBanner("Push Completed", fmt.Sprintf("Successfully exported and pushed %s (%s) to registry %s/%s.", pushAppID, pushArch, pushRegistry, pushOCIRepository))
		return nil
	},
}

func init() {
	RootCmd.AddCommand(pushOCICmd)

	pushOCICmd.Flags().StringVar(&pushAppID, "app", "", "app ID (reverse-DNS format)")
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
}
