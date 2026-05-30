package plan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "manifest-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("Valid JSON with id", func(t *testing.T) {
		path := filepath.Join(tempDir, "app.json")
		content := `{"id": "org.flatpak.TestApp", "runtime": "org.gnome.Platform"}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		m, err := ParseManifest(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.ID != "org.flatpak.TestApp" {
			t.Errorf("expected ID 'org.flatpak.TestApp', got %q", m.ID)
		}
		if m.Runtime != "org.gnome.Platform" {
			t.Errorf("expected Runtime 'org.gnome.Platform', got %q", m.Runtime)
		}
	})

	t.Run("Valid JSON with app-id", func(t *testing.T) {
		path := filepath.Join(tempDir, "app-id.json")
		content := `{"app-id": "org.flatpak.TestApp2", "runtime": "org.gnome.Platform"}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		m, err := ParseManifest(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.ID != "org.flatpak.TestApp2" {
			t.Errorf("expected ID 'org.flatpak.TestApp2', got %q", m.ID)
		}
	})

	t.Run("Valid YAML", func(t *testing.T) {
		path := filepath.Join(tempDir, "app.yaml")
		content := `
id: org.flatpak.TestAppYaml
runtime: org.gnome.PlatformYaml
`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		m, err := ParseManifest(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.ID != "org.flatpak.TestAppYaml" {
			t.Errorf("expected ID 'org.flatpak.TestAppYaml', got %q", m.ID)
		}
		if m.Runtime != "org.gnome.PlatformYaml" {
			t.Errorf("expected Runtime 'org.gnome.PlatformYaml', got %q", m.Runtime)
		}
	})

	t.Run("Missing ID", func(t *testing.T) {
		path := filepath.Join(tempDir, "missing-id.json")
		content := `{"runtime": "org.gnome.Platform"}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := ParseManifest(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("Missing Runtime", func(t *testing.T) {
		path := filepath.Join(tempDir, "missing-runtime.json")
		content := `{"id": "org.flatpak.TestApp"}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := ParseManifest(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("File Not Found", func(t *testing.T) {
		_, err := ParseManifest(filepath.Join(tempDir, "does-not-exist.json"))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
