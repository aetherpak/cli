package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func resetCleanFlags() {
	cleanYes = false
	cleanCCache = false
	cleanState = false
	cleanPreview = false
	cleanSite = false
	cleanRecords = false
	cleanRepo = false

	cleanCCacheDir = ".ccache"
	cleanStateDir = ".state"
	cleanPreviewDir = "_preview"
	cleanSiteDir = "_site"
	cleanRecordsDir = "records"
	cleanRepoPath = "repo"

	viper.Reset()

	for _, flag := range []string{
		"yes", "confirm", "ccache", "state", "preview", "site", "records", "repo",
		"ccache-dir", "state-dir", "preview-dir", "site-dir", "records-dir", "repo-path",
	} {
		if f := cleanCmd.Flags().Lookup(flag); f != nil {
			f.Changed = false
			_ = f.Value.Set(f.DefValue)
		}
	}
}

func TestCleanCmdNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	resetCleanFlags()

	// In non-interactive mode with nothing to delete, it should return nil (success)
	err := cleanCmd.RunE(cleanCmd, nil)
	if err != nil {
		t.Fatalf("expected no error when nothing to clean, got: %v", err)
	}
}

func TestCleanCmdAll(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	resetCleanFlags()

	// Create all target directories
	dirs := []string{".ccache", ".state", "_preview", "_site", "records", "repo"}
	for _, d := range dirs {
		if err := os.Mkdir(d, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", d, err)
		}
	}

	_ = cleanCmd.Flags().Set("yes", "true")
	initConfig()
	bindFlags(cleanCmd)

	err := cleanCmd.RunE(cleanCmd, nil)
	if err != nil {
		t.Fatalf("expected no error running clean: %v", err)
	}

	// Verify all are deleted
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil || !os.IsNotExist(err) {
			t.Errorf("expected directory %s to be deleted, but it still exists", d)
		}
	}
}

func TestCleanCmdFilter(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	resetCleanFlags()

	// Create directories
	dirs := []string{".ccache", ".state", "_preview"}
	for _, d := range dirs {
		if err := os.Mkdir(d, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", d, err)
		}
	}

	_ = cleanCmd.Flags().Set("yes", "true")
	_ = cleanCmd.Flags().Set("ccache", "true")
	_ = cleanCmd.Flags().Set("preview", "true")
	initConfig()
	bindFlags(cleanCmd)

	err := cleanCmd.RunE(cleanCmd, nil)
	if err != nil {
		t.Fatalf("expected no error running clean: %v", err)
	}

	// Verify .ccache and _preview are deleted, but .state remains
	if _, err := os.Stat(".ccache"); err == nil || !os.IsNotExist(err) {
		t.Error("expected .ccache to be deleted")
	}
	if _, err := os.Stat("_preview"); err == nil || !os.IsNotExist(err) {
		t.Error("expected _preview to be deleted")
	}
	if _, err := os.Stat(".state"); err != nil {
		t.Error("expected .state to remain")
	}
}

func TestCleanCmdNonInteractiveNoYes(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	resetCleanFlags()

	if err := os.Mkdir(".ccache", 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	initConfig()
	bindFlags(cleanCmd)

	err := cleanCmd.RunE(cleanCmd, nil)
	if err == nil {
		t.Fatal("expected error in non-interactive run without --yes flag")
	}
	if !strings.Contains(err.Error(), "confirmation required") {
		t.Errorf("expected confirmation required error, got: %v", err)
	}

	// Verify directory was NOT deleted
	if _, err := os.Stat(".ccache"); err != nil {
		t.Error("expected .ccache to remain when confirmation fails")
	}
}

func TestCleanCmdConfigOverrides(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	resetCleanFlags()

	// Write custom configuration
	configData := []byte(`
output_dir: "my_outputs"
defaults:
  ccache_dir: "custom_ccache"
  state_dir: "custom_state"
apps:
  - id: "org.test.App"
    manifest: "org.test.App.yaml"
    ccache_dir: "app_ccache"
    state_dir: "app_state"
`)
	if err := os.WriteFile("aetherpak.yaml", configData, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create directories
	dirs := []string{"custom_ccache", "custom_state", "app_ccache", "app_state", "my_outputs/_site", "my_outputs/records", "my_outputs/repo"}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", d, err)
		}
	}

	_ = cleanCmd.Flags().Set("yes", "true")
	initConfig()
	bindFlags(cleanCmd)

	err := cleanCmd.RunE(cleanCmd, nil)
	if err != nil {
		t.Fatalf("expected no error running clean: %v", err)
	}

	// Verify all configured/overridden directories are deleted
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil || !os.IsNotExist(err) {
			t.Errorf("expected directory %s to be deleted, but it still exists", d)
		}
	}
}
