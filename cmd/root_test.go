package cmd

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/spf13/viper"
)

func TestConfigEnvBindings(t *testing.T) {
	// Extract all expected configuration keys via reflection
	expectedKeys := extractKeys(reflect.TypeOf(config.Config{}), "")

	if len(expectedKeys) == 0 {
		t.Fatal("failed to extract any configuration keys via reflection")
	}

	// Reset viper and initialize env bindings
	viper.Reset()
	initConfig()

	for _, key := range expectedKeys {
		// Construct expected environment variable name matching viper config
		// replacer rules: SetEnvPrefix("AETHERPAK") and replace "." and "-" with "_"
		envKey := "AETHERPAK_" + strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(key))
		testVal := "test-value-for-" + key

		// Temporarily set the environment variable
		if err := os.Setenv(envKey, testVal); err != nil {
			t.Fatalf("failed to set env var %s: %v", envKey, err)
		}

		// Retrieve from viper
		gotVal := viper.GetString(key)

		// Clean up env var
		_ = os.Unsetenv(envKey)

		if gotVal != testVal {
			t.Errorf("Config field %q is not bound to environment variable %s (expected %q, got %q)", key, envKey, testVal, gotVal)
		}
	}
}

// extractKeys recursively traverses a struct type to find all mapstructure tagged keys.
func extractKeys(t reflect.Type, prefix string) []string {
	var keys []string
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag == "" || tag == "-" || tag == "apps" || tag == "channel_mappings" {
			continue
		}
		// Clean tag options (e.g. omitempty)
		parts := strings.Split(tag, ",")
		tagKey := parts[0]

		keyPath := tagKey
		if prefix != "" {
			keyPath = prefix + "." + tagKey
		}

		ft := field.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		if ft.Kind() == reflect.Struct {
			keys = append(keys, extractKeys(ft, keyPath)...)
		} else if keyPath != "defaults.remotes" && keyPath != "defaults.flatpaks" {
			keys = append(keys, keyPath)
		}
	}
	return keys
}

func TestBindInheritedFlags(t *testing.T) {
	viper.Reset()
	viper.Set("config", "my-custom-config-path.yaml")

	bindFlags(statusCmd)

	f := statusCmd.Flag("config")
	if f == nil {
		t.Fatal("expected statusCmd to have 'config' flag (inherited from RootCmd)")
	}

	if f.Value.String() != "my-custom-config-path.yaml" {
		t.Errorf("expected inherited flag '--config' to be bound to Viper value 'my-custom-config-path.yaml', got: %q", f.Value.String())
	}
}
