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

func TestResolveAll(t *testing.T) {
	tmp := t.TempDir()
	headsDir := filepath.Join(tmp, "refs", "heads")
	refPath1 := filepath.Join(headsDir, "app", "org.example.TestApp1", "x86_64", "stable")
	refPath2 := filepath.Join(headsDir, "app", "org.example.TestApp2", "aarch64", "beta")
	if err := os.MkdirAll(filepath.Dir(refPath1), 0755); err != nil {
		t.Fatalf("failed to create temp dirs: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(refPath2), 0755); err != nil {
		t.Fatalf("failed to create temp dirs: %v", err)
	}
	if err := os.WriteFile(refPath1, []byte("dummy-commit-sha1"), 0644); err != nil {
		t.Fatalf("failed to write mock ref file 1: %v", err)
	}
	if err := os.WriteFile(refPath2, []byte("dummy-commit-sha2"), 0644); err != nil {
		t.Fatalf("failed to write mock ref file 2: %v", err)
	}

	infos, err := ResolveAll(tmp)
	if err != nil {
		t.Fatalf("ResolveAll failed: %v", err)
	}

	if len(infos) != 2 {
		t.Fatalf("expected 2 resolved infos, got %d", len(infos))
	}

	var found1, found2 bool
	for _, info := range infos {
		if info.AppID == "org.example.TestApp1" && info.Arch == "x86_64" && info.Branch == "stable" {
			found1 = true
		}
		if info.AppID == "org.example.TestApp2" && info.Arch == "aarch64" && info.Branch == "beta" {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Fatalf("did not resolve both apps correctly: %+v", infos)
	}
}
