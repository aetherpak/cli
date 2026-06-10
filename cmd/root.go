package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	cfgFile   string
	verbose   bool
	jsonLog   bool
	plain     bool
	noColor   bool
	logFile   string
	outputDir string
)

// Version holds the version tag of the AetherPak CLI, injected at build time.
var Version = "dev"

// RootCmd represents the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:     "aetherpak",
	Version: Version,
	Short:   "AetherPak Core CLI is a tool for building, pushing and releasing Flatpak apps as OCI images",
	Long: `AetherPak Core CLI replaces scripting pipelines for converting flatpak
applications into OCI hosted repositories on GHCR with deployment sites on Pages.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		bindFlags(cmd)

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
	cliChangedFlags = make(map[*cobra.Command]map[string]bool)
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
	RootCmd.PersistentFlags().StringVar(&outputDir, "output-dir", "", "base directory for all output assets")

	// Bind flags to viper
	viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("json-log", RootCmd.PersistentFlags().Lookup("json-log"))
	viper.BindPFlag("plain", RootCmd.PersistentFlags().Lookup("plain"))
	viper.BindPFlag("no-color", RootCmd.PersistentFlags().Lookup("no-color"))
	viper.BindPFlag("log-file", RootCmd.PersistentFlags().Lookup("log-file"))
	viper.BindPFlag("output_dir", RootCmd.PersistentFlags().Lookup("output-dir"))
}

func initConfig() {
	viper.SetEnvPrefix("AETHERPAK")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	// Bind custom non-prefixed env vars for OCI credentials
	_ = viper.BindEnv("oci_username", "OCI_USERNAME")
	_ = viper.BindEnv("oci_password", "OCI_PASSWORD")

	// Bind all configuration keys to support environment variable overrides in zero-config mode
	for _, key := range []string{
		"registry", "pages_url", "oci_repository", "remote_name", "no_sign",
		"repo_title", "repo_homepage", "runtime_repo", "output_dir",
		"linter.strict", "linter.ignore_rules", "linter.exceptions", "linter.exceptions_file",
		"branding.logo_url", "branding.favicon_url", "branding.accent_color", "branding.footer_text", "branding.index_template",
		"defaults.ccache", "defaults.ccache_dir", "defaults.state_dir", "defaults.run_linter", "defaults.builder_args",
		"defaults.no_install_deps", "defaults.no_flathub",
	} {
		_ = viper.BindEnv(key)
	}

	if vCfgFile := viper.GetString("config"); vCfgFile != "" {
		viper.SetConfigFile(vCfgFile)
	} else {
		for _, ext := range []string{"yaml", "yml"} {
			if _, err := os.Stat("aetherpak." + ext); err == nil {
				viper.SetConfigFile("aetherpak." + ext)
				break
			}
		}
	}
}

// LoadConfig reads and parses the optional configuration file.
// If the configuration file is missing and was not explicitly requested, it returns a default config.
func LoadConfig() (*config.Config, error) {
	var cfg config.Config

	if vCfgFile := viper.GetString("config"); vCfgFile != "" {
		viper.SetConfigFile(vCfgFile)
	} else {
		found := false
		for _, ext := range []string{"yaml", "yml"} {
			if _, err := os.Stat("aetherpak." + ext); err == nil {
				viper.SetConfigFile("aetherpak." + ext)
				found = true
				break
			}
		}
		if !found {
			if err := viper.Unmarshal(&cfg, viper.DecodeHook(
				mapstructure.ComposeDecodeHookFunc(
					mapstructure.StringToTimeDurationHookFunc(),
					mapstructure.StringToSliceHookFunc(","),
					config.RemoteConfigDecodeHook(),
				),
			)); err != nil {
				return nil, fmt.Errorf("failed to unmarshal config via viper: %w", err)
			}
			if vOutputDir := viper.GetString("output_dir"); vOutputDir != "" {
				cfg.OutputDir = vOutputDir
			}
			cfg.Normalize()
			return &cfg, nil
		}
	}

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

	if err := viper.Unmarshal(&cfg, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			config.RemoteConfigDecodeHook(),
		),
	)); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config via viper: %w", err)
	}

	if vOutputDir := viper.GetString("output_dir"); vOutputDir != "" {
		cfg.OutputDir = vOutputDir
	}

	// Normalize global defaults and app overrides
	cfg.Normalize()

	// Validate apps
	for i := range cfg.Apps {
		if err := cfg.Apps[i].Validate(); err != nil {
			return nil, fmt.Errorf("invalid application config at index %d: %w", i, err)
		}
	}

	// Validate channel mappings patterns
	for pattern := range cfg.ChannelMappings {
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return nil, fmt.Errorf("invalid channel mapping pattern %q: %w", pattern, err)
		}
	}

	return &cfg, nil
}

var cliChangedFlags = make(map[*cobra.Command]map[string]bool)

// IsFlagExplicitlySet checks if a flag was explicitly set on the command line
// (or set before bindFlags populated it from Viper).
func IsFlagExplicitlySet(cmd *cobra.Command, name string) bool {
	if m, ok := cliChangedFlags[cmd]; ok {
		return m[name]
	}
	return cmd.Flags().Changed(name)
}

// bindFlags automatically populates flags from viper (config file or env vars)
// if they were not explicitly set on the command line.
func bindFlags(cmd *cobra.Command) {
	cliChangedFlags[cmd] = make(map[string]bool)
	visited := make(map[string]bool)

	bind := func(f *pflag.Flag) {
		if visited[f.Name] {
			return
		}
		visited[f.Name] = true

		if f.Changed {
			cliChangedFlags[cmd][f.Name] = true
		} else {
			viperKey := strings.ReplaceAll(f.Name, "-", "_")
			if viper.IsSet(viperKey) {
				val := viper.Get(viperKey)
				if val != nil {
					var valStr string
					switch v := val.(type) {
					case []string:
						valStr = strings.Join(v, ",")
					case []interface{}:
						strList := make([]string, len(v))
						for i, item := range v {
							strList[i] = fmt.Sprintf("%v", item)
						}
						valStr = strings.Join(strList, ",")
					default:
						valStr = fmt.Sprintf("%v", v)
					}
					if f.Value.String() != valStr {
						_ = f.Value.Set(valStr)
					}
				}
			}
		}
	}

	cmd.Flags().VisitAll(bind)
	cmd.InheritedFlags().VisitAll(bind)
}
