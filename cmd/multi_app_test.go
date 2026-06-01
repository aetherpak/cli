package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/spf13/viper"
)

func TestValidateArch(t *testing.T) {
	if err := config.ValidateArch("x86_64"); err != nil {
		t.Errorf("expected x86_64 to be valid, got: %v", err)
	}
	if err := config.ValidateArch("aarch64"); err != nil {
		t.Errorf("expected aarch64 to be valid, got: %v", err)
	}
	if err := config.ValidateArch(""); err != nil {
		t.Errorf("expected empty string to be valid, got: %v", err)
	}
	if err := config.ValidateArch("invalid"); err == nil {
		t.Error("expected invalid arch to fail")
	}
}

func TestRemoteNameFallback(t *testing.T) {
	cfg := &config.Config{
		RemoteName: "myorg/myrepo",
	}
	cfg.Normalize()
	if cfg.OCIRepository != "myorg/myrepo" {
		t.Errorf("expected OCIRepository to fall back to RemoteName, got %q", cfg.OCIRepository)
	}
}

func TestBuildInvalidArch(t *testing.T) {
	_ = buildCmd.Flags().Set("arch", "invalid-arch")
	_ = buildCmd.Flags().Set("manifest", "nonexistent")

	err := buildCmd.RunE(buildCmd, nil)
	if err == nil {
		t.Error("expected error when validating architecture")
	} else if !strings.Contains(err.Error(), "unsupported architecture") {
		t.Errorf("expected error about unsupported architecture, got: %v", err)
	}
}

func TestPublishInvalidArch(t *testing.T) {
	_ = publishCmd.Flags().Set("arch", "invalid-arch")

	err := publishCmd.RunE(publishCmd, nil)
	if err == nil {
		t.Error("expected error when validating architecture")
	} else if !strings.Contains(err.Error(), "unsupported architecture") {
		t.Errorf("expected error about unsupported architecture, got: %v", err)
	}
}

func TestPushOCIInvalidArch(t *testing.T) {
	_ = pushOCICmd.Flags().Set("arch", "invalid-arch")

	err := pushOCICmd.RunE(pushOCICmd, nil)
	if err == nil {
		t.Error("expected error when validating architecture")
	} else if !strings.Contains(err.Error(), "unsupported architecture") {
		t.Errorf("expected error about unsupported architecture, got: %v", err)
	}
}

func TestImportInvalidArch(t *testing.T) {
	_ = importCmd.Flags().Set("arch", "invalid-arch")

	err := importCmd.RunE(importCmd, nil)
	if err == nil {
		t.Error("expected error when validating architecture")
	} else if !strings.Contains(err.Error(), "unsupported architecture") {
		t.Errorf("expected error about unsupported architecture, got: %v", err)
	}
}

func TestPublishMissingConfigGracefulError(t *testing.T) {
	viper.Reset()
	_ = publishCmd.Flags().Set("app-id", "")
	_ = publishCmd.Flags().Set("arch", "x86_64")

	err := publishCmd.RunE(publishCmd, nil)
	if err == nil {
		t.Error("expected error when config is missing and no app-id is provided")
	} else if !strings.Contains(err.Error(), "no application ID provided and no configuration file found") {
		t.Errorf("expected graceful error message, got: %v", err)
	}
}

func TestBuildMissingConfigGracefulError(t *testing.T) {
	viper.Reset()
	_ = buildCmd.Flags().Set("app-id", "")
	_ = buildCmd.Flags().Set("manifest", "")
	_ = buildCmd.Flags().Set("arch", "x86_64")

	// Ensure manifest flag is not marked as changed
	buildCmd.Flags().Lookup("manifest").Changed = false

	err := buildCmd.RunE(buildCmd, nil)
	if err == nil {
		t.Error("expected error when config is missing and no app-id/manifest is provided")
	} else if !strings.Contains(err.Error(), "no manifest provided and no configuration file found") {
		t.Errorf("expected graceful error message, got: %v", err)
	}
}

func TestPushOCIMissingConfigGracefulError(t *testing.T) {
	viper.Reset()
	_ = pushOCICmd.Flags().Set("app-id", "")
	_ = pushOCICmd.Flags().Set("arch", "x86_64")

	err := pushOCICmd.RunE(pushOCICmd, nil)
	if err == nil {
		t.Error("expected error when config is missing and no app-id is provided")
	} else if !strings.Contains(err.Error(), "no application ID provided and no configuration file found") {
		t.Errorf("expected graceful error message, got: %v", err)
	}
}

func TestImportMissingConfigGracefulError(t *testing.T) {
	viper.Reset()
	_ = importCmd.Flags().Set("app-id", "")
	_ = importCmd.Flags().Set("bundle-url", "")
	_ = importCmd.Flags().Set("bundle-path", "")
	_ = importCmd.Flags().Set("arch", "x86_64")

	err := importCmd.RunE(importCmd, nil)
	if err == nil {
		t.Error("expected error when config is missing and no app-id/bundle is provided")
	} else if !strings.Contains(err.Error(), "either bundle-url or bundle-path is required") {
		t.Errorf("expected graceful error message, got: %v", err)
	}
}

func TestPlanManifestForceWarning(t *testing.T) {
	manifestContent := []byte(`{
		"id": "org.example.App",
		"runtime": "org.gnome.Platform",
		"runtime-version": "45"
	}`)
	err := os.WriteFile("temp_manifest.json", manifestContent, 0644)
	if err != nil {
		t.Fatalf("failed to write temp manifest: %v", err)
	}
	defer os.Remove("temp_manifest.json")

	viper.Reset()
	_ = planCmd.Flags().Set("manifest", "temp_manifest.json")
	_ = planCmd.Flags().Set("force", "org.example.App")
	_ = planCmd.Flags().Set("arch", "x86_64")

	defer func() {
		planManifest = ""
		forceFlag = ""
		planArches = nil
		planBranch = ""
		planCmd.Flags().Lookup("manifest").Changed = false
		planCmd.Flags().Lookup("force").Changed = false
		planCmd.Flags().Lookup("arch").Changed = false
	}()

	err = planCmd.RunE(planCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error running planCmd: %v", err)
	}

	if forceFlag != "" {
		t.Errorf("expected forceFlag to be cleared when manifest is set, got %q", forceFlag)
	}
}

func TestResolveChannelConfigError(t *testing.T) {
	err := os.WriteFile("aetherpak.yaml", []byte("invalid_yaml: [unclosed list"), 0644)
	if err != nil {
		t.Fatalf("failed to write bad config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	viper.Reset()
	initConfig()

	err = resolveChannelCmd.RunE(resolveChannelCmd, nil)
	if err == nil {
		t.Error("expected error when resolving channel with broken configuration, got nil")
	} else if !strings.Contains(err.Error(), "Configuration error:") {
		t.Errorf("expected Configuration error, got: %v", err)
	}
}

func TestBuildSiteConfigError(t *testing.T) {
	err := os.WriteFile("aetherpak.yaml", []byte("invalid_yaml: [unclosed list"), 0644)
	if err != nil {
		t.Fatalf("failed to write bad config: %v", err)
	}
	defer os.Remove("aetherpak.yaml")

	viper.Reset()
	initConfig()

	err = buildSiteCmd.RunE(buildSiteCmd, nil)
	if err == nil {
		t.Error("expected error when building site with broken configuration, got nil")
	} else if !strings.Contains(err.Error(), "Configuration error:") {
		t.Errorf("expected Configuration error, got: %v", err)
	}
}
