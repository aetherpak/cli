package cmd

import (
	"fmt"
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
	logFile string
)

// RootCmd represents the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:   "aetherpak",
	Short: "AetherPak Core CLI is a tool for building, pushing and releasing Flatpak apps as OCI images",
	Long: `AetherPak Core CLI replaces scripting pipelines for converting flatpak
applications into OCI hosted repositories on GHCR with deployment sites on Pages.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		vVerbose := viper.GetBool("verbose")
		vJSONLog := viper.GetBool("json-log")
		vPlain := viper.GetBool("plain") || viper.GetBool("no-color")
		vLogFile := viper.GetString("log-file")

		logger.Init(vVerbose, vJSONLog, vPlain)
		if err := logger.InitFileLogging(vLogFile); err != nil {
			return NewCmdErrorf(1, "failed to initialize logging: %w", err)
		}
		logger.Debug("Logger initialized with verbose=%v json=%v plain=%v", vVerbose, vJSONLog, vPlain)
		return nil
	},
	SilenceErrors: true,
	SilenceUsage:  true,
}

// CmdError represents a command execution error containing an exit status code.
type CmdError struct {
	Err  error
	Code int
}

func (e *CmdError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *CmdError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewCmdError wraps an error with an exit code.
func NewCmdError(code int, err error) *CmdError {
	return &CmdError{Err: err, Code: code}
}

// NewCmdErrorf wraps a formatted error message with an exit code.
func NewCmdErrorf(code int, format string, args ...interface{}) *CmdError {
	return &CmdError{Err: fmt.Errorf(format, args...), Code: code}
}

// Execute adds all child commands to the root command and sets flags appropriately.
// It returns the process exit code (0 on success).
func Execute() int {
	err := RootCmd.Execute()
	hasError := err != nil

	if hasError {
		logger.ErrorBanner("Execution Failure", err.Error())
	}

	logger.CloseLogFile(hasError)

	if hasError {
		if cmdErr, ok := err.(*CmdError); ok {
			return cmdErr.Code
		}
		return 1
	}
	return 0
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
	RootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "write logs to the specified file (retains logs on success)")

	// Bind flags to viper
	viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("json-log", RootCmd.PersistentFlags().Lookup("json-log"))
	viper.BindPFlag("plain", RootCmd.PersistentFlags().Lookup("plain"))
	viper.BindPFlag("no-color", RootCmd.PersistentFlags().Lookup("no-color"))
	viper.BindPFlag("log-file", RootCmd.PersistentFlags().Lookup("log-file"))
}

func initConfig() {
	viper.SetEnvPrefix("AETHERPAK")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	// Bind custom non-prefixed env vars for OCI credentials
	_ = viper.BindEnv("oci_username", "OCI_USERNAME")
	_ = viper.BindEnv("oci_password", "OCI_PASSWORD")

	if vCfgFile := viper.GetString("config"); vCfgFile != "" {
		viper.SetConfigFile(vCfgFile)
	} else {
		// Look for aetherpak.yaml or aetherpak.yml in current working directory
		viper.AddConfigPath(".")
		viper.SetConfigName("aetherpak")
	}
	viper.SetConfigType("yaml")
}

// LoadConfig reads and parses the optional configuration file.
// If the configuration file is missing and was not explicitly requested, it returns a default config.
func LoadConfig() (*config.Config, error) {
	var cfg config.Config

	if vCfgFile := viper.GetString("config"); vCfgFile != "" {
		viper.SetConfigFile(vCfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("aetherpak")
	}
	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			if cfgFile != "" {
				return nil, fmt.Errorf("configured file %q not found: %w", cfgFile, err)
			}
			logger.Debug("Configuration file not found, proceeding in zero-config.")
		} else {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	} else {
		logger.Debug("Using config file: %s", viper.ConfigFileUsed())
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
