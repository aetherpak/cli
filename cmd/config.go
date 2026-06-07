package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
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

		switch v := val.(type) {
		case string:
			fmt.Fprintln(cmd.OutOrStdout(), v)
		default:
			fmt.Fprintf(cmd.OutOrStdout(), "%v\n", v)
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

func init() {
	RootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}
