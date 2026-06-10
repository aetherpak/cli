package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestPreviewCmdRespectConfigTemplate(t *testing.T) {
	t.Chdir(t.TempDir())

	// Write a minimal config with a nonexistent index_template
	configData := []byte(`
repo_title: "My CLI Preview Test"
pages_url: "https://pages.test"
branding:
  index_template: "nonexistent_template_should_be_respected.html"
`)
	err := os.WriteFile("aetherpak.yaml", configData, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	siteDir := filepath.Join(t.TempDir(), "preview-out")

	defer func() {
		previewTemplatePath = ""
		previewLive = false
		previewLiveURL = ""
		previewGPG = false
		previewApps = ""
		previewSiteDir = ""
		previewServe = false
		previewPort = 0
		previewRemoteName = ""
		previewRepoTitle = ""
		previewRepoHP = ""
		previewDefaultTemplate = false
		previewCmd.Flags().Lookup("serve").Changed = false
		previewCmd.Flags().Lookup("site-dir").Changed = false
		previewCmd.Flags().Lookup("apps").Changed = false
		previewCmd.Flags().Lookup("gpg").Changed = false
		previewCmd.Flags().Lookup("default-template").Changed = false
	}()

	viper.Reset()
	previewTemplatePath = ""
	previewLive = false
	previewLiveURL = ""
	previewGPG = false
	previewApps = ""
	previewSiteDir = ""
	previewServe = false
	previewPort = 0
	previewRemoteName = ""
	previewRepoTitle = ""
	previewRepoHP = ""
	previewDefaultTemplate = false
	previewCmd.Flags().Lookup("serve").Changed = false
	previewCmd.Flags().Lookup("site-dir").Changed = false
	previewCmd.Flags().Lookup("apps").Changed = false
	previewCmd.Flags().Lookup("gpg").Changed = false
	previewCmd.Flags().Lookup("default-template").Changed = false

	_ = previewCmd.Flags().Set("serve", "false")
	_ = previewCmd.Flags().Set("site-dir", siteDir)
	_ = previewCmd.Flags().Set("apps", "single")
	_ = previewCmd.Flags().Set("gpg", "false")

	initConfig()
	bindFlags(previewCmd)

	err = previewCmd.RunE(previewCmd, nil)
	if err == nil {
		t.Fatal("expected error since preview should respect custom config template which is nonexistent")
	}
	if !strings.Contains(err.Error(), "failed to read custom index template") {
		t.Errorf("expected failed to read custom index template error, got: %v", err)
	}
}

func TestPreviewCmdForceDefaultTemplate(t *testing.T) {
	t.Chdir(t.TempDir())

	// Write a minimal config with a nonexistent index_template
	configData := []byte(`
repo_title: "My CLI Preview Test"
pages_url: "https://pages.test"
branding:
  index_template: "nonexistent_template_should_be_ignored.html"
`)
	err := os.WriteFile("aetherpak.yaml", configData, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	siteDir := filepath.Join(t.TempDir(), "preview-out")

	defer func() {
		previewTemplatePath = ""
		previewLive = false
		previewLiveURL = ""
		previewGPG = false
		previewApps = ""
		previewSiteDir = ""
		previewServe = false
		previewPort = 0
		previewRemoteName = ""
		previewRepoTitle = ""
		previewRepoHP = ""
		previewDefaultTemplate = false
		previewCmd.Flags().Lookup("serve").Changed = false
		previewCmd.Flags().Lookup("site-dir").Changed = false
		previewCmd.Flags().Lookup("apps").Changed = false
		previewCmd.Flags().Lookup("gpg").Changed = false
		previewCmd.Flags().Lookup("default-template").Changed = false
	}()

	viper.Reset()
	previewTemplatePath = ""
	previewLive = false
	previewLiveURL = ""
	previewGPG = false
	previewApps = ""
	previewSiteDir = ""
	previewServe = false
	previewPort = 0
	previewRemoteName = ""
	previewRepoTitle = ""
	previewRepoHP = ""
	previewDefaultTemplate = false
	previewCmd.Flags().Lookup("serve").Changed = false
	previewCmd.Flags().Lookup("site-dir").Changed = false
	previewCmd.Flags().Lookup("apps").Changed = false
	previewCmd.Flags().Lookup("gpg").Changed = false
	previewCmd.Flags().Lookup("default-template").Changed = false

	_ = previewCmd.Flags().Set("serve", "false")
	_ = previewCmd.Flags().Set("site-dir", siteDir)
	_ = previewCmd.Flags().Set("apps", "single")
	_ = previewCmd.Flags().Set("gpg", "false")
	_ = previewCmd.Flags().Set("default-template", "true")

	initConfig()
	bindFlags(previewCmd)

	err = previewCmd.RunE(previewCmd, nil)
	if err != nil {
		t.Fatalf("failed to run previewCmd: %v", err)
	}

	indexPath := filepath.Join(siteDir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Errorf("expected index.html to be created at %s", indexPath)
	}
}
