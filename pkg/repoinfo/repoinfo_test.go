package repoinfo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
)

func TestParseRef(t *testing.T) {
	// app ref
	refType, id, arch, branch, err := parseRef("app/org.example.App/x86_64/stable")
	if err != nil || refType != "app" || id != "org.example.App" || arch != "x86_64" || branch != "stable" {
		t.Fatalf("got %q %q %q %q err=%v", refType, id, arch, branch, err)
	}
	// runtime ref
	refType, id, arch, branch, err = parseRef("runtime/org.freedesktop.Sdk.Extension.xrt/x86_64/stable")
	if err != nil || refType != "runtime" || id != "org.freedesktop.Sdk.Extension.xrt" || arch != "x86_64" || branch != "stable" {
		t.Fatalf("got %q %q %q %q err=%v", refType, id, arch, branch, err)
	}
	// invalid type prefix
	if _, _, _, _, err := parseRef("not/an/app/ref"); err == nil {
		t.Fatal("expected error for invalid ref type prefix")
	}
	// malformed (too few parts)
	if _, _, _, _, err := parseRef("app/too/few"); err == nil {
		t.Fatal("expected error for malformed ref")
	}
}

func TestInfoRef(t *testing.T) {
	info := Info{AppID: "org.example.App", Arch: "x86_64", Branch: "stable", RefType: "app"}
	if got := info.Ref(); got != "app/org.example.App/x86_64/stable" {
		t.Fatalf("Info.Ref() = %q, want app/org.example.App/x86_64/stable", got)
	}
	info.RefType = "runtime"
	if got := info.Ref(); got != "runtime/org.example.App/x86_64/stable" {
		t.Fatalf("Info.Ref() = %q, want runtime/org.example.App/x86_64/stable", got)
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

	info, err := Resolve(nil, tmp)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if info.AppID != "org.example.TestApp" || info.Arch != "x86_64" || info.Branch != "stable" || info.RefType != "app" {
		t.Fatalf("resolved incorrect info: %+v", info)
	}
}

func TestResolveRuntime(t *testing.T) {
	tmp := t.TempDir()
	headsDir := filepath.Join(tmp, "refs", "heads")
	refPath := filepath.Join(headsDir, "runtime", "org.freedesktop.Sdk.Extension.xrt", "x86_64", "stable")
	if err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {
		t.Fatalf("failed to create temp dirs: %v", err)
	}
	if err := os.WriteFile(refPath, []byte("dummy-commit-sha"), 0644); err != nil {
		t.Fatalf("failed to write mock ref file: %v", err)
	}

	info, err := Resolve(nil, tmp)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if info.AppID != "org.freedesktop.Sdk.Extension.xrt" || info.Arch != "x86_64" || info.Branch != "stable" || info.RefType != "runtime" {
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

	infos, err := ResolveAll(nil, tmp)
	if err != nil {
		t.Fatalf("ResolveAll failed: %v", err)
	}

	if len(infos) != 2 {
		t.Fatalf("expected 2 resolved infos, got %d", len(infos))
	}

	var found1, found2 bool
	for _, info := range infos {
		if info.AppID == "org.example.TestApp1" && info.Arch == "x86_64" && info.Branch == "stable" && info.RefType == "app" {
			found1 = true
		}
		if info.AppID == "org.example.TestApp2" && info.Arch == "aarch64" && info.Branch == "beta" && info.RefType == "app" {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Fatalf("did not resolve both apps correctly: %+v", infos)
	}
}

func TestResolveAllMixed(t *testing.T) {
	tmp := t.TempDir()
	headsDir := filepath.Join(tmp, "refs", "heads")
	appRef := filepath.Join(headsDir, "app", "org.example.TestApp", "x86_64", "stable")
	runtimeRef := filepath.Join(headsDir, "runtime", "org.freedesktop.Sdk.Extension.xrt", "x86_64", "stable")
	if err := os.MkdirAll(filepath.Dir(appRef), 0755); err != nil {
		t.Fatalf("failed to create temp dirs: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(runtimeRef), 0755); err != nil {
		t.Fatalf("failed to create temp dirs: %v", err)
	}
	if err := os.WriteFile(appRef, []byte("dummy-commit-sha1"), 0644); err != nil {
		t.Fatalf("failed to write mock app ref: %v", err)
	}
	if err := os.WriteFile(runtimeRef, []byte("dummy-commit-sha2"), 0644); err != nil {
		t.Fatalf("failed to write mock runtime ref: %v", err)
	}

	infos, err := ResolveAll(nil, tmp)
	if err != nil {
		t.Fatalf("ResolveAll failed: %v", err)
	}

	if len(infos) != 2 {
		t.Fatalf("expected 2 resolved infos, got %d", len(infos))
	}

	var foundApp, foundRuntime bool
	for _, info := range infos {
		if info.AppID == "org.example.TestApp" && info.RefType == "app" {
			foundApp = true
		}
		if info.AppID == "org.freedesktop.Sdk.Extension.xrt" && info.RefType == "runtime" {
			foundRuntime = true
		}
	}

	if !foundApp || !foundRuntime {
		t.Fatalf("did not resolve both app and runtime refs: %+v", infos)
	}
}

func TestResolveOstreeFallback(t *testing.T) {
	mockExec := executil.NewMockExecutor()
	mockExec.OutMap["ostree"] = []byte("app/org.example.MockFromOstree/x86_64/stable\nruntime/org.gnome.Platform/x86_64/45\n")

	tmp := t.TempDir() // empty directory, Walk won't find any ref

	// 1. Test Resolve fallback
	info, err := Resolve(mockExec, tmp)
	if err != nil {
		t.Fatalf("Resolve fallback failed: %v", err)
	}
	if info.AppID != "org.example.MockFromOstree" || info.Arch != "x86_64" || info.Branch != "stable" || info.RefType != "app" {
		t.Fatalf("unexpected Resolve fallback result: %+v", info)
	}

	// 2. Test ResolveAll fallback
	infos, err := ResolveAll(mockExec, tmp)
	if err != nil {
		t.Fatalf("ResolveAll fallback failed: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 resolved infos from fallback, got %d: %+v", len(infos), infos)
	}

	var foundApp, foundRuntime bool
	for _, i := range infos {
		if i.AppID == "org.example.MockFromOstree" && i.RefType == "app" {
			foundApp = true
		}
		if i.AppID == "org.gnome.Platform" && i.RefType == "runtime" {
			foundRuntime = true
		}
	}
	if !foundApp || !foundRuntime {
		t.Fatalf("unexpected ResolveAll fallback result: %+v", infos)
	}
}

func TestRestoreEmptyDirs(t *testing.T) {
	tmp := t.TempDir()
	// Initially, folders refs/heads, refs/mirrors, refs/remotes do not exist.
	subdirs := []string{
		filepath.Join(tmp, "refs", "heads"),
		filepath.Join(tmp, "refs", "mirrors"),
		filepath.Join(tmp, "refs", "remotes"),
	}
	for _, dir := range subdirs {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("directory %q already exists before restoration", dir)
		}
	}

	// Run RestoreEmptyDirs
	if err := RestoreEmptyDirs(tmp); err != nil {
		t.Fatalf("RestoreEmptyDirs failed: %v", err)
	}

	// Verify they now exist
	for _, dir := range subdirs {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			t.Fatalf("directory %q was not restored properly", dir)
		}
	}
}
