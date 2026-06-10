package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/executil"
	"github.com/aetherpak/aetherpak/pkg/status"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	statusGPGKeys       []string
	statusGPGPassphrase string
	statusJSON          bool
	statusExecutor      executil.Executor = executil.NewOSExecutor()
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Checks system dependencies, configuration files, and signing credentials",
	Long: `Status validates that all required system executables (flatpak, flatpak-builder, ostree)
are available, parses the repository configuration, and checks the status of GPG signing keys and credentials.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Attempt to load the configuration
		cfg, err := LoadConfig()

		resolvedPath := cfgFile
		if resolvedPath == "" {
			resolvedPath = "aetherpak.yaml"
			if _, err := os.Stat("aetherpak.yml"); err == nil {
				resolvedPath = "aetherpak.yml"
			}
		}

		// Resolve keys from command-line flags (keys will already contain GPG keys from flag or env var)
		keys := statusGPGKeys

		// Run diagnostics
		var statusCfg *config.Config
		if err == nil && viper.ConfigFileUsed() != "" {
			statusCfg = cfg
		}

		report := status.Check(
			statusExecutor,
			statusCfg,
			err,
			resolvedPath,
			keys,
			[]byte(statusGPGPassphrase),
		)

		hasMissingRequired := false
		for _, dep := range report.Dependencies {
			if dep.Required && !dep.Found {
				hasMissingRequired = true
				break
			}
		}

		if statusJSON {
			bz, mErr := json.MarshalIndent(report, "", "  ")
			if mErr != nil {
				return NewCmdErrorf(1, "failed to serialize status JSON: %w", mErr)
			}
			fmt.Println(string(bz))
			if hasMissingRequired {
				return NewCmdErrorf(1, "missing required system dependency")
			}
			return nil
		}

		status.PrintReport(os.Stdout, report)
		if hasMissingRequired {
			return NewCmdErrorf(1, "missing required system dependency")
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(statusCmd)

	statusCmd.Flags().StringSliceVar(&statusGPGKeys, "gpg-key", nil, "GPG private key block(s) or path(s) to verify signing setup")
	statusCmd.Flags().StringVar(&statusGPGPassphrase, "gpg-key-passphrase", "", "passphrase unlocking the GPG private key(s) to check")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output raw diagnostics status as JSON for script parsing")
}
