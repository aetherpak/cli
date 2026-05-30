//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestE2EPlanManifest(t *testing.T) {
	// Compile binary
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = ".."
	var buildStderr bytes.Buffer
	buildCmd.Stderr = &buildStderr
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to compile aetherpak binary (%v): %s", err, buildStderr.String())
	}
	binaryPath := filepath.Join("..", "bin", "aetherpak")

	tempDir, err := os.MkdirTemp("", "plan-manifest-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manifestPath := filepath.Join(tempDir, "org.flatpak.DemoApp.json")
	manifestContent := `{"id": "org.flatpak.DemoApp", "runtime": "org.freedesktop.Platform", "runtime-version": "23.08"}`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	t.Run("Plan with manifest, custom arch and branch", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "plan",
			"--manifest="+manifestPath,
			"--arch=x86_64",
			"--arch=aarch64",
			"--branch=beta",
			"--output=json",
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("plan failed: %v, stderr: %s", err, stderr.String())
		}

		type MatrixRow struct {
			Source         string `json:"source"`
			AppID          string `json:"app-id"`
			Manifest       string `json:"manifest"`
			Runtime        string `json:"runtime"`
			RuntimeVersion string `json:"runtime-version"`
			Branch         string `json:"branch"`
			Arch           string `json:"arch"`
			Runner         string `json:"runner"`
			RunLinter      bool   `json:"run-linter"`
		}

		type PlanResult struct {
			Apps   []string    `json:"apps"`
			Matrix []MatrixRow `json:"matrix"`
		}

		var res PlanResult
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatalf("failed to parse stdout: %v, raw stdout: %s", err, stdout.String())
		}

		if len(res.Apps) != 1 || res.Apps[0] != "org.flatpak.DemoApp" {
			t.Errorf("unexpected selected apps: %v", res.Apps)
		}

		if len(res.Matrix) != 2 {
			t.Fatalf("expected 2 matrix rows, got %d", len(res.Matrix))
		}

		// Check row 0
		r0 := res.Matrix[0]
		if r0.AppID != "org.flatpak.DemoApp" || r0.Source != "manifest" || r0.Manifest != manifestPath {
			t.Errorf("row 0 mismatch: %+v", r0)
		}
		if r0.Runtime != "org.freedesktop.Platform" || r0.RuntimeVersion != "23.08" || r0.Branch != "beta" || r0.Arch != "x86_64" {
			t.Errorf("row 0 details mismatch: %+v", r0)
		}

		// Check row 1
		r1 := res.Matrix[1]
		if r1.AppID != "org.flatpak.DemoApp" || r1.Source != "manifest" || r1.Manifest != manifestPath {
			t.Errorf("row 1 mismatch: %+v", r1)
		}
		if r1.Runtime != "org.freedesktop.Platform" || r1.RuntimeVersion != "23.08" || r1.Branch != "beta" || r1.Arch != "aarch64" {
			t.Errorf("row 1 details mismatch: %+v", r1)
		}
	})

	t.Run("Plan with manifest, defaulting arch and branch", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "plan",
			"--manifest="+manifestPath,
			"--output=json",
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("plan failed: %v, stderr: %s", err, stderr.String())
		}

		type MatrixRow struct {
			Source   string `json:"source"`
			AppID    string `json:"app-id"`
			Manifest string `json:"manifest"`
			Runtime  string `json:"runtime"`
			Branch   string `json:"branch"`
			Arch     string `json:"arch"`
		}

		type PlanResult struct {
			Apps   []string    `json:"apps"`
			Matrix []MatrixRow `json:"matrix"`
		}

		var res PlanResult
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatalf("failed to parse stdout: %v, raw: %s", err, stdout.String())
		}

		if len(res.Matrix) != 1 {
			t.Fatalf("expected 1 matrix row, got %d", len(res.Matrix))
		}

		r0 := res.Matrix[0]
		if r0.Arch != "x86_64" || r0.Branch != "stable" {
			t.Errorf("default mappings mismatch: %+v", r0)
		}
	})

	t.Run("Plan with manifest defaults to run-linter=true", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "plan",
			"--manifest="+manifestPath,
			"--output=json",
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("plan failed: %v, stderr: %s", err, stderr.String())
		}

		type MatrixRow struct {
			RunLinter bool `json:"run-linter"`
		}

		type PlanResult struct {
			Matrix []MatrixRow `json:"matrix"`
		}

		var res PlanResult
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatalf("failed to parse stdout: %v, raw: %s", err, stdout.String())
		}

		if len(res.Matrix) != 1 {
			t.Fatalf("expected 1 matrix row, got %d", len(res.Matrix))
		}

		if !res.Matrix[0].RunLinter {
			t.Error("expected run-linter to default to true for manifests, got false")
		}
	})

	t.Run("Plan with manifest and disable-linter=true", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "plan",
			"--manifest="+manifestPath,
			"--disable-linter=true",
			"--output=json",
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("plan failed: %v, stderr: %s", err, stderr.String())
		}

		type MatrixRow struct {
			RunLinter bool `json:"run-linter"`
		}

		type PlanResult struct {
			Matrix []MatrixRow `json:"matrix"`
		}

		var res PlanResult
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatalf("failed to parse stdout: %v, raw: %s", err, stdout.String())
		}

		if len(res.Matrix) != 1 {
			t.Fatalf("expected 1 matrix row, got %d", len(res.Matrix))
		}

		if res.Matrix[0].RunLinter {
			t.Error("expected run-linter to be false when --disable-linter is specified, got true")
		}
	})

	t.Run("Plan with both manifest and force should fail", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "plan",
			"--manifest="+manifestPath,
			"--force=all",
			"--output=json",
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			t.Fatal("expected plan command to fail when both --manifest and --force are supplied, but it succeeded")
		}
	})

	t.Run("Plan with manifest containing invalid ID should fail validation", func(t *testing.T) {
		badManifestPath := filepath.Join(tempDir, "bad-id.json")
		badContent := `{"id": "invalid ID with spaces", "runtime": "org.freedesktop.Platform"}`
		if err := os.WriteFile(badManifestPath, []byte(badContent), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		cmd := exec.Command(binaryPath, "plan",
			"--manifest="+badManifestPath,
			"--output=json",
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			t.Fatal("expected validation to fail for invalid manifest ID, but it succeeded")
		}
	})

	t.Run("Plan with invalid branch flag should fail validation", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "plan",
			"--manifest="+manifestPath,
			"--branch=invalid branch with spaces",
			"--output=json",
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			t.Fatal("expected validation to fail for invalid branch flag, but it succeeded")
		}
	})
}
