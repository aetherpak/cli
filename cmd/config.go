package cmd

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage aetherpak configuration",
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Ensure config is loaded so viper is populated
		_, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}

		key := args[0]
		val := viper.Get(key)
		if val == nil {
			// Print nothing and exit with 0 if key is not found
			return nil
		}

		// Check if it is a complex structure (slice, array, map)
		isComplex := false
		if val != nil {
			rv := reflect.ValueOf(val)
			k := rv.Kind()
			if k == reflect.Slice || k == reflect.Array || k == reflect.Map {
				isComplex = true
			}
		}

		if isComplex {
			yamlData, err := yaml.Marshal(val)
			if err != nil {
				if logger.IsPlain() {
					fmt.Fprintf(cmd.OutOrStdout(), "%v\n", val)
				} else {
					valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
					fmt.Fprintln(cmd.OutOrStdout(), valStyle.Render(fmt.Sprintf("%v", val)))
				}
			} else {
				yamlStr := string(yamlData)
				if logger.IsPlain() {
					fmt.Fprint(cmd.OutOrStdout(), yamlStr)
				} else {
					valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
					lines := strings.Split(strings.TrimRight(yamlStr, "\n"), "\n")
					for _, line := range lines {
						fmt.Fprintln(cmd.OutOrStdout(), valStyle.Render(line))
					}
				}
			}
			return nil
		}

		if logger.IsPlain() {
			switch v := val.(type) {
			case string:
				fmt.Fprintln(cmd.OutOrStdout(), v)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "%v\n", v)
			}
			return nil
		}

		// Rich formatted output matching show command styling
		valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		boolTrueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
		boolFalseStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))

		switch v := val.(type) {
		case bool:
			if v {
				fmt.Fprintln(cmd.OutOrStdout(), boolTrueStyle.Render("true"))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), boolFalseStyle.Render("false"))
			}
		case string:
			fmt.Fprintln(cmd.OutOrStdout(), valStyle.Render(v))
		default:
			fmt.Fprintln(cmd.OutOrStdout(), valStyle.Render(fmt.Sprintf("%v", v)))
		}
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		var parsedValue interface{} = value
		if value == "true" {
			parsedValue = true
		} else if value == "false" {
			parsedValue = false
		} else if valInt, err := strconv.Atoi(value); err == nil {
			parsedValue = valInt
		}

		configPath := cfgFile
		if configPath == "" {
			configPath = "aetherpak.yaml"
			if _, err := os.Stat("aetherpak.yml"); err == nil {
				configPath = "aetherpak.yml"
			}
		}

		data, err := os.ReadFile(configPath)
		m := make(map[string]interface{})
		if err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to read config file: %w", err)
			}
		} else {
			if err := yaml.Unmarshal(data, &m); err != nil {
				return fmt.Errorf("failed to parse config file: %w", err)
			}
		}

		if err := setNestedKey(m, key, parsedValue); err != nil {
			return err
		}

		out, err := yaml.Marshal(m)
		if err != nil {
			return fmt.Errorf("failed to marshal config to YAML: %w", err)
		}

		if err := os.WriteFile(configPath, out, 0644); err != nil {
			return fmt.Errorf("failed to write config file: %w", err)
		}

		return nil
	},
}

func setNestedKey(m map[string]interface{}, keyPath string, value interface{}) error {
	parts := strings.Split(keyPath, ".")
	curr := m
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		next, exists := curr[part]
		if !exists {
			nextMap := make(map[string]interface{})
			curr[part] = nextMap
			curr = nextMap
		} else {
			// Convert generic map to map[string]interface{} if needed
			switch val := next.(type) {
			case map[string]interface{}:
				curr = val
			case map[interface{}]interface{}:
				// Sometimes yaml unmarshals as map[interface{}]interface{}
				strMap := make(map[string]interface{})
				for k, v := range val {
					strMap[fmt.Sprintf("%v", k)] = v
				}
				curr[part] = strMap
				curr = strMap
			default:
				nextMap := make(map[string]interface{})
				curr[part] = nextMap
				curr = nextMap
			}
		}
	}
	leaf := parts[len(parts)-1]
	curr[leaf] = value
	return nil
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the configuration in the resolved state",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}

		// Gather active environment overrides
		var envOverrides []string
		for _, env := range os.Environ() {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := parts[0]
			val := parts[1]
			if strings.HasPrefix(key, "AETHERPAK_") || key == "OCI_USERNAME" || key == "OCI_PASSWORD" {
				if key == "OCI_PASSWORD" || key == "AETHERPAK_OCI_PASSWORD" {
					val = "********"
				}
				envOverrides = append(envOverrides, fmt.Sprintf("%s=%s", key, val))
			}
		}

		// Gather active CLI flag overrides
		var flagOverrides []string
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Changed {
				flagOverrides = append(flagOverrides, fmt.Sprintf("--%s=%s", f.Name, f.Value.String()))
			}
		})

		// If in plain mode (CI, --plain, --no-color), output structured YAML
		if logger.IsPlain() {
			yamlData, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("failed to marshal resolved configuration to YAML: %w", err)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "--- Resolved Configuration ---")
			fmt.Fprint(w, string(yamlData))
			fmt.Fprintln(w)
			fmt.Fprintln(w, "--- Active Overrides ---")
			if len(envOverrides) == 0 && len(flagOverrides) == 0 {
				fmt.Fprintln(w, "No overrides in effect.")
			} else {
				for _, env := range envOverrides {
					fmt.Fprintf(w, "- Environment: %s\n", env)
				}
				for _, flag := range flagOverrides {
					fmt.Fprintf(w, "- CLI Flag:    %s\n", flag)
				}
			}
			return nil
		}

		// Otherwise, render a premium rich Lipgloss interface
		w := cmd.OutOrStdout()
		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
		sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("36"))
		keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		boolTrueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
		boolFalseStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
		dimmedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
		borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))

		printKV := func(key string, val interface{}) {
			var valStr string
			switch v := val.(type) {
			case bool:
				if v {
					valStr = boolTrueStyle.Render("true")
				} else {
					valStr = boolFalseStyle.Render("false")
				}
			case string:
				if v == "" {
					valStr = dimmedStyle.Render("(empty)")
				} else {
					valStr = valStyle.Render(v)
				}
			case int:
				valStr = valStyle.Render(strconv.Itoa(v))
			case []string:
				if len(v) == 0 {
					valStr = dimmedStyle.Render("(none)")
				} else {
					valStr = valStyle.Render(fmt.Sprintf("[%s]", strings.Join(v, ", ")))
				}
			default:
				valStr = valStyle.Render(fmt.Sprintf("%v", v))
			}
			fmt.Fprintf(w, "  %-18s %s\n", dimmedStyle.Render(key+":"), valStr)
		}

		fmt.Fprintln(w, titleStyle.Render("AETHERPAK RESOLVED CONFIGURATION"))
		fmt.Fprintln(w, borderStyle.Render(strings.Repeat("─", 45)))
		fmt.Fprintln(w)

		// 1. Global Settings
		fmt.Fprintln(w, sectionStyle.Render("GLOBAL SETTINGS"))
		printKV("registry", cfg.Registry)
		printKV("oci_repository", cfg.OCIRepository)
		printKV("pages_url", cfg.PagesURL)
		printKV("remote_name", cfg.RemoteName)
		printKV("output_dir", cfg.OutputDir)
		printKV("no_sign", cfg.NoSign)
		printKV("repo_title", cfg.RepoTitle)
		printKV("repo_homepage", cfg.RepoHomepage)
		printKV("runtime_repo", cfg.RuntimeRepo)
		fmt.Fprintln(w)

		// 2. Linter Config
		fmt.Fprintln(w, sectionStyle.Render("LINTER CONFIG"))
		if cfg.Linter != nil {
			var strictVal bool
			if cfg.Linter.Strict != nil {
				strictVal = *cfg.Linter.Strict
			}
			printKV("strict", strictVal)
			printKV("ignore_rules", cfg.Linter.IgnoreRules)
			printKV("exceptions", cfg.Linter.Exceptions)
			printKV("exceptions_file", cfg.Linter.ExceptionsFile)
		} else {
			fmt.Fprintln(w, "  "+dimmedStyle.Render("(no linter configuration)"))
		}
		fmt.Fprintln(w)

		// 3. Branding
		fmt.Fprintln(w, sectionStyle.Render("BRANDING"))
		if cfg.Branding != nil {
			printKV("logo_url", cfg.Branding.LogoURL)
			printKV("favicon_url", cfg.Branding.FaviconURL)
			printKV("accent_color", cfg.Branding.AccentColor)
			printKV("footer_text", cfg.Branding.FooterText)
			printKV("index_template", cfg.Branding.IndexTemplate)
		} else {
			fmt.Fprintln(w, "  "+dimmedStyle.Render("(no branding configuration)"))
		}
		fmt.Fprintln(w)

		// 4. Defaults
		fmt.Fprintln(w, sectionStyle.Render("DEFAULTS"))
		if cfg.Defaults != nil {
			var ccacheVal bool
			if cfg.Defaults.CCache != nil {
				ccacheVal = *cfg.Defaults.CCache
			}
			printKV("ccache", ccacheVal)
			printKV("ccache_dir", cfg.Defaults.CCacheDir)
			printKV("state_dir", cfg.Defaults.StateDir)
			printKV("run_linter", cfg.Defaults.RunLinter)
			printKV("builder_args", cfg.Defaults.BuilderArgs)

			if len(cfg.Defaults.Remotes) > 0 {
				var remotes []string
				for k, v := range cfg.Defaults.Remotes {
					remotes = append(remotes, fmt.Sprintf("%s=%s", k, v.String()))
				}
				printKV("remotes", remotes)
			} else {
				printKV("remotes", []string{})
			}

			if len(cfg.Defaults.Flatpaks) > 0 {
				var deps []string
				for _, dep := range cfg.Defaults.Flatpaks {
					deps = append(deps, fmt.Sprintf("%s:%s", dep.Remote, dep.Ref))
				}
				printKV("flatpaks", deps)
			} else {
				printKV("flatpaks", []string{})
			}
		} else {
			fmt.Fprintln(w, "  "+dimmedStyle.Render("(no defaults configuration)"))
		}
		fmt.Fprintln(w)

		// 5. Channel Mappings
		fmt.Fprintln(w, sectionStyle.Render("CHANNEL MAPPINGS"))
		if len(cfg.ChannelMappings) > 0 {
			for k, v := range cfg.ChannelMappings {
				printKV(k, v)
			}
		} else {
			fmt.Fprintln(w, "  "+dimmedStyle.Render("(no channel mappings configured)"))
		}
		fmt.Fprintln(w)

		// 6. Applications
		appHeader := fmt.Sprintf("APPLICATIONS (%d app%s)", len(cfg.Apps), func() string {
			if len(cfg.Apps) != 1 {
				return "s"
			}
			return ""
		}())
		fmt.Fprintln(w, sectionStyle.Render(appHeader))
		if len(cfg.Apps) > 0 {
			for _, app := range cfg.Apps {
				appTitle := fmt.Sprintf("• %s [%s] (%s)", app.ID, app.Branch, strings.Join(app.Arches, ", "))
				fmt.Fprintln(w, "  "+keyStyle.Render(appTitle))
				if app.Manifest != "" {
					fmt.Fprintf(w, "    %-16s %s\n", dimmedStyle.Render("manifest:"), valStyle.Render(app.Manifest))
				} else if len(app.Bundles) > 0 {
					fmt.Fprintf(w, "    %-16s %s\n", dimmedStyle.Render("bundles:"), dimmedStyle.Render(fmt.Sprintf("(%d bundle architectures)", len(app.Bundles))))
				}

				var ccacheStr string
				if app.CCache != nil {
					if *app.CCache {
						ccacheStr = boolTrueStyle.Render("true")
					} else {
						ccacheStr = boolFalseStyle.Render("false")
					}
				} else {
					ccacheStr = dimmedStyle.Render("(inherited)")
				}

				fmt.Fprintf(w, "    %-16s %s\n", dimmedStyle.Render("run_linter:"), func() string {
					if app.RunLinter {
						return boolTrueStyle.Render("true")
					}
					return boolFalseStyle.Render("false")
				}())
				fmt.Fprintf(w, "    %-16s %s\n", dimmedStyle.Render("ccache:"), ccacheStr)
				fmt.Fprintf(w, "    %-16s %s\n", dimmedStyle.Render("ccache_dir:"), valStyle.Render(app.CCacheDir))
				fmt.Fprintf(w, "    %-16s %s\n", dimmedStyle.Render("state_dir:"), valStyle.Render(app.StateDir))
			}
		} else {
			fmt.Fprintln(w, "  "+dimmedStyle.Render("(no applications configured)"))
		}
		fmt.Fprintln(w)

		// 7. Active Overrides
		fmt.Fprintln(w, sectionStyle.Render("ACTIVE OVERRIDES"))
		if len(envOverrides) == 0 && len(flagOverrides) == 0 {
			fmt.Fprintln(w, "  "+dimmedStyle.Render("No overrides in effect."))
		} else {
			warningStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
			fmt.Fprintln(w, "  "+warningStyle.Render("⚠ The following environment variables or CLI flags are in effect:"))
			for _, env := range envOverrides {
				fmt.Fprintf(w, "  • %-12s %s\n", dimmedStyle.Render("Environment:"), valStyle.Render(env))
			}
			for _, flag := range flagOverrides {
				fmt.Fprintf(w, "  • %-12s %s\n", dimmedStyle.Render("CLI Flag:"), valStyle.Render(flag))
			}
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configShowCmd)
}
