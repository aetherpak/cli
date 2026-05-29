package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	verbose bool
	jsonLog bool
	plain   bool
	noColor bool
)

// RootCmd represents the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:   "aetherpak",
	Short: "AetherPak Core CLI is a tool for building, pushing and releasing Flatpak apps as OCI images",
	Long: `AetherPak Core CLI replaces scripting pipelines for converting flatpak 
applications into OCI hosted repositories on GHCR with deployment sites on Pages.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		isPlain := plain || noColor
		logger.Init(verbose, jsonLog, isPlain)
		logger.Debug("Logger initialized with verbose=%v json=%v plain=%v", verbose, jsonLog, isPlain)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		logger.ErrorBanner("Execution Failure", err.Error())
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		renderPremiumHelp(c, c.OutOrStdout())
	})

	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is aetherpak.yaml or aetherpak.yml)")
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging")
	RootCmd.PersistentFlags().BoolVar(&jsonLog, "json-log", false, "enable JSON formatted logging")
	RootCmd.PersistentFlags().BoolVar(&plain, "plain", false, "disable colors and fancy formatting (plain text output)")
	RootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colors and fancy formatting (alias for --plain)")

	// Bind flags to viper
	viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("json-log", RootCmd.PersistentFlags().Lookup("json-log"))
	viper.BindPFlag("plain", RootCmd.PersistentFlags().Lookup("plain"))
	viper.BindPFlag("no-color", RootCmd.PersistentFlags().Lookup("no-color"))
}

func initConfig() {
	viper.SetEnvPrefix("AETHERPAK")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Look for aetherpak.yaml or aetherpak.yml in current working directory
		viper.AddConfigPath(".")
		viper.SetConfigName("aetherpak")
		viper.SetConfigType("yaml")
	}

	if err := viper.ReadInConfig(); err == nil {
		logger.Debug("Using config file: %s", viper.ConfigFileUsed())
	}
}

// LoadConfig reads and parses the optional configuration file.
// If the configuration file is missing and was not explicitly requested, it returns a default config.
func LoadConfig() (*config.Config, error) {
	var cfg config.Config

	cfgPath := viper.ConfigFileUsed()
	if cfgPath == "" && cfgFile != "" {
		cfgPath = cfgFile
	}

	if cfgPath == "" {
		// Attempt to resolve default name
		defaultPath := "aetherpak.yaml"
		if _, err := os.Stat(defaultPath); err == nil {
			cfgPath = defaultPath
		} else if _, err := os.Stat("aetherpak.yml"); err == nil {
			cfgPath = "aetherpak.yml"
		}
	}

	if cfgPath != "" {
		absPath, err := filepath.Abs(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve absolute path of config file: %w", err)
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			// If explicitly requested file is missing, return error
			if cfgFile != "" {
				return nil, fmt.Errorf("configured file %q not found: %w", cfgFile, err)
			}
			logger.Debug("Configuration file %s unreadable or missing, proceeding in zero-config.", cfgPath)
		} else {
			viper.SetConfigType("yaml")
			if err := viper.ReadConfig(bytes.NewBuffer(data)); err != nil {
				return nil, fmt.Errorf("failed to load config into viper: %w", err)
			}
		}
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config via viper: %w", err)
	}

	// Normalize global defaults and app overrides
	cfg.Normalize()

	// Validate apps
	for i := range cfg.Apps {
		if err := cfg.Apps[i].Validate(); err != nil {
			return nil, fmt.Errorf("invalid application config at index %d: %w", i, err)
		}
	}

	return &cfg, nil
}
