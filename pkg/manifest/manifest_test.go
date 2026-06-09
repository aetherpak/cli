package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

func TestParseManifestYAML(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "org.example.App.yaml",
		"app-id: org.example.App\nruntime: org.freedesktop.Platform\nruntime-version: \"24.08\"\nbranch: \"25.08\"\n")
	m, err := ParseManifest(p)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.ID != "org.example.App" {
		t.Errorf("ID = %q, want org.example.App", m.ID)
	}
	if m.Runtime != "org.freedesktop.Platform" || m.RuntimeVersion != "24.08" {
		t.Errorf("runtime fields = %q/%q", m.Runtime, m.RuntimeVersion)
	}
	if m.Branch != "25.08" {
		t.Errorf("Branch = %q, want 25.08", m.Branch)
	}
}

func TestParseManifestJSON(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "m.json", `{"id":"org.example.App","runtime":"org.kde.Platform"}`)
	m, err := ParseManifest(p)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.ID != "org.example.App" {
		t.Errorf("ID = %q", m.ID)
	}
}

func TestParseManifestJSONAppID(t *testing.T) {
	// app-id is the dominant key in real Flatpak manifests; verify the JSON path.
	dir := t.TempDir()
	p := writeFile(t, dir, "m.json", `{"app-id":"org.example.App","runtime":"org.kde.Platform"}`)
	m, err := ParseManifest(p)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.ID != "org.example.App" {
		t.Errorf("ID = %q, want org.example.App", m.ID)
	}
}

func TestParseManifestMissingID(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "m.yaml", "runtime: org.freedesktop.Platform\n")
	if _, err := ParseManifest(p); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestDetectInDirSingleCandidate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "org.example.App.yml",
		"app-id: org.example.App\nruntime: org.freedesktop.Platform\n")
	writeFile(t, dir, "README.md", "hello")
	got, err := DetectInDir(dir)
	if err != nil {
		t.Fatalf("DetectInDir: %v", err)
	}
	if got != "org.example.App.yml" {
		t.Errorf("got %q, want org.example.App.yml", got)
	}
}

func TestDetectInDirNone(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "hello")
	if _, err := DetectInDir(dir); err == nil {
		t.Fatal("expected error when no manifest candidate")
	}
}

func TestDetectInDirIgnoresNonManifestWithID(t *testing.T) {
	// A YAML/JSON file that has an 'id' but no runtime/sdk is not a manifest.
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", "id: some.random.thing\nname: not a manifest\n")
	writeFile(t, dir, "org.example.App.yaml",
		"app-id: org.example.App\nruntime: org.freedesktop.Platform\n")
	got, err := DetectInDir(dir)
	if err != nil {
		t.Fatalf("DetectInDir: %v", err)
	}
	if got != "org.example.App.yaml" {
		t.Errorf("got %q, want org.example.App.yaml (non-manifest id file must be ignored)", got)
	}
}

func TestDetectInDirSdkQualifies(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.json", `{"app-id":"org.example.App","sdk":"org.freedesktop.Sdk"}`)
	got, err := DetectInDir(dir)
	if err != nil {
		t.Fatalf("DetectInDir: %v", err)
	}
	if got != "app.json" {
		t.Errorf("got %q, want app.json", got)
	}
}

func TestDetectInDirAmbiguous(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yaml", "app-id: a\nruntime: r\n")
	writeFile(t, dir, "b.yaml", "app-id: b\nruntime: r\n")
	if _, err := DetectInDir(dir); err == nil {
		t.Fatal("expected error for ambiguous candidates")
	}
}
