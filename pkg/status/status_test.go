package status

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/executil"
)

func TestCheckDependenciesAllFound(t *testing.T) {
	mockExec := executil.NewMockExecutor()
	mockExec.PathMap["flatpak"] = "/usr/bin/flatpak"
	mockExec.OutMap["flatpak"] = []byte("Flatpak 1.14.4\nSome other metadata info")
	mockExec.PathMap["flatpak-builder"] = "/usr/bin/flatpak-builder"
	mockExec.OutMap["flatpak-builder"] = []byte("flatpak-builder 1.2.3")
	mockExec.PathMap["ostree"] = "/usr/bin/ostree"
	mockExec.OutMap["ostree"] = []byte("libostree 2023.2")
	mockExec.PathMap["flatpak-builder-lint"] = "/usr/bin/flatpak-builder-lint"
	mockExec.OutMap["flatpak-builder-lint"] = []byte("flatpak-builder-lint 0.1.0")
	mockExec.PathMap["podman"] = "/usr/bin/podman"
	mockExec.OutMap["podman"] = []byte("podman version 4.9.4")
	mockExec.PathMap["docker"] = "/usr/bin/docker"
	mockExec.OutMap["docker"] = []byte("Docker version 26.0.0")

	report := Check(mockExec, nil, nil, "", nil, nil)

	foundCount := 0
	for _, dep := range report.Dependencies {
		if dep.Found {
			foundCount++
		}
	}
	if foundCount != 6 {
		t.Errorf("expected all 6 dependencies to be found, got %d", foundCount)
	}

	// Verify versions are cleaned up to first line
	for _, dep := range report.Dependencies {
		if dep.Name == "flatpak" && dep.Version != "Flatpak 1.14.4" {
			t.Errorf("expected flatpak version 'Flatpak 1.14.4', got %q", dep.Version)
		}
	}
}

func TestCheckDependenciesFallbackLintAndRecommendation(t *testing.T) {
	mockExec := executil.NewMockExecutor()
	mockExec.PathMap["flatpak"] = "/usr/bin/flatpak"
	mockExec.OutMap["flatpak"] = []byte("Flatpak 1.14.4")
	mockExec.PathMap["ostree"] = "/usr/bin/ostree"
	mockExec.OutMap["ostree"] = []byte("libostree 2023.2")
	mockExec.PathMap["podman"] = "/usr/bin/podman"
	mockExec.OutMap["podman"] = []byte("podman version 4.9.4")

	// Verify that flatpak-builder-lint version call via flatpak is mocked
	mockExec.OnCommand = func(cmd *executil.MockCommand) {
		if cmd.Name == "flatpak" && len(cmd.Args) > 0 && cmd.Args[0] == "run" {
			cmd.OutData = []byte("flatpak-builder-lint 0.2.0")
		}
	}

	report := Check(mockExec, nil, nil, "", nil, nil)

	// flatpak, ostree, podman, and fallback flatpak-builder-lint should be found
	// flatpak-builder and docker should NOT be found
	found := make(map[string]bool)
	for _, dep := range report.Dependencies {
		if dep.Found {
			found[dep.Name] = true
		}
	}

	if !found["flatpak"] || !found["ostree"] || !found["podman"] || !found["flatpak-builder-lint"] {
		t.Errorf("expected flatpak, ostree, podman, and fallback flatpak-builder-lint to be found, got %+v", found)
	}

	if found["flatpak-builder"] || found["docker"] {
		t.Errorf("expected flatpak-builder and docker to NOT be found, got %+v", found)
	}

	// Verify recommendation formatting trigger
	var buf bytes.Buffer
	PrintReport(&buf, report)
	output := buf.String()

	if !strings.Contains(output, "Recommendation:") {
		t.Errorf("expected recommendation to be output when flatpak-builder is missing but podman is present")
	}
	if !strings.Contains(output, "podman run --privileged") {
		t.Errorf("expected podman recommendations, got: %s", output)
	}
}

func TestCheckConfigValidation(t *testing.T) {
	cfg := &config.Config{
		Registry:   "registry.example.com",
		PagesURL:   "https://pages.example.com",
		RemoteName: "myremote",
		Apps: []config.App{
			{
				ID:       "org.example.App1",
				Manifest: "manifest1.json",
			},
			{
				ID: "org.example.App2",
				// Bundle-based app
			},
		},
	}

	// Test 1: Config error passing
	cErr := errors.New("malformed yaml syntax")
	report := Check(nil, nil, cErr, "aetherpak.yaml", nil, nil)
	if report.ConfigLoaded {
		t.Error("expected config loaded to be false when error is provided")
	}
	if report.ConfigError != cErr {
		t.Errorf("expected config error %v, got %v", cErr, report.ConfigError)
	}

	// Test 2: App manifest checking. One app has manifest but it doesn't exist on disk.
	report2 := Check(nil, cfg, nil, "aetherpak.yaml", nil, nil)
	if !report2.ConfigLoaded {
		t.Error("expected config loaded to be true")
	}
	if report2.Registry != "registry.example.com" {
		t.Errorf("expected registry registry.example.com, got %q", report2.Registry)
	}

	var buf bytes.Buffer
	PrintReport(&buf, report2)
	output := buf.String()

	if !strings.Contains(output, "org.example.App1") {
		t.Error("expected output to contain org.example.App1 description")
	}
	if !strings.Contains(output, "Error: Manifest file not found:") {
		t.Error("expected manifest existence error in output")
	}
}

func TestCheckGPGSigning(t *testing.T) {
	// 1. Signing disabled
	cfg := &config.Config{
		NoSign: true,
	}
	report := Check(nil, cfg, nil, "aetherpak.yaml", nil, nil)
	if report.SigningEnabled {
		t.Error("expected signing to be reported as disabled")
	}

	// 2. Signing enabled but no keys loaded
	cfg.NoSign = false
	report2 := Check(nil, cfg, nil, "aetherpak.yaml", nil, nil)
	if !report2.SigningEnabled {
		t.Error("expected signing to be reported as enabled")
	}
	if report2.GPGKeysCount != 0 {
		t.Errorf("expected 0 keys loaded, got %d", report2.GPGKeysCount)
	}

	// 3. Signing enabled with invalid GPG keys
	report3 := Check(nil, cfg, nil, "aetherpak.yaml", []string{"invalid-key-data"}, nil)
	if report3.GPGKeysCount != 1 {
		t.Errorf("expected 1 key loaded, got %d", report3.GPGKeysCount)
	}
	if report3.SigningError == nil {
		t.Error("expected parsing error for invalid GPG keys")
	}
}
