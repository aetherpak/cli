package repoinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRef(t *testing.T) {
	id, arch, branch, err := parseRef("app/org.example.App/x86_64/stable")
	if err != nil || id != "org.example.App" || arch != "x86_64" || branch != "stable" {
		t.Fatalf("got %q %q %q err=%v", id, arch, branch, err)
	}
	if _, _, _, err := parseRef("not/an/app/ref"); err == nil {
		t.Fatal("expected error for non-app ref")
	}
	if _, _, _, err := parseRef("app/too/few"); err == nil {
		t.Fatal("expected error for malformed ref")
	}
}

func TestResolve(t *testing.T) {
	tmp := t.TempDir()
	headsDir := filepath.Join(tmp, "refs", "heads")
	refPath := filepath.Join(headsDir, "app", "org.example.TestApp", "x86_64", "stable")
	if err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {
		t.Fatalf("failed to create temp dirs: %v", err)
	}
	if err := os.WriteFile(refPath, []byte("dummy-commit-sha"), 0644); err != nil {
		t.Fatalf("failed to write mock ref file: %v", err)
	}

	info, err := Resolve(tmp)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if info.AppID != "org.example.TestApp" || info.Arch != "x86_64" || info.Branch != "stable" {
		t.Fatalf("resolved incorrect info: %+v", info)
	}
}
