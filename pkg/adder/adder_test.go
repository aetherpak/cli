package adder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
	"github.com/aetherpak/aetherpak/pkg/gitutil"
)

func writeManifest(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "org.example.App.yaml")
	if err := os.WriteFile(p, []byte("app-id: org.example.App\nruntime: org.freedesktop.Platform\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunManifestWritesConfig(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeManifest(t, dir)
	cfgPath := filepath.Join(dir, "aetherpak.yaml")

	err := Run(Options{
		ConfigPath:   cfgPath,
		Source:       SourceManifest,
		ManifestPath: manifestPath,
		Confirm:      true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, _ := os.ReadFile(cfgPath)
	s := string(data)
	if !strings.Contains(s, "org.example.App") {
		t.Errorf("config missing app:\n%s", s)
	}
	// Manifest must be stored relative to the config dir (not the absolute temp path).
	if !strings.Contains(s, "manifest: org.example.App.yaml") {
		t.Errorf("manifest not relativized:\n%s", s)
	}
}

func TestRunManifestAppliesToggles(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeManifest(t, dir)
	cfgPath := filepath.Join(dir, "aetherpak.yaml")

	err := Run(Options{
		ConfigPath:   cfgPath,
		Source:       SourceManifest,
		ManifestPath: manifestPath,
		Confirm:      true,
		Toggles:      map[string]bool{"run-linter": true, "ccache": true, "install-deps-from-flathub": true},
		BuilderArgs:  []string{"--verbose"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	s := string(mustRead(t, cfgPath))
	if !strings.Contains(s, "run-linter: true") {
		t.Errorf("run-linter not written:\n%s", s)
	}
	if !strings.Contains(s, "ccache: true") {
		t.Errorf("ccache not written:\n%s", s)
	}
	if !strings.Contains(s, "--install-deps-from=flathub") || !strings.Contains(s, "--verbose") {
		t.Errorf("builder args not written:\n%s", s)
	}
}

func TestRunManifestMissingIDError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(p, []byte("runtime: org.freedesktop.Platform\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := Run(Options{
		ConfigPath:   filepath.Join(dir, "aetherpak.yaml"),
		Source:       SourceManifest,
		ManifestPath: p,
		Confirm:      true,
	})
	if err == nil || !strings.Contains(err.Error(), "did not contain a valid app id") {
		t.Fatalf("expected 'did not contain a valid app id' error, got: %v", err)
	}
}

func TestRunGitDefaultSubmodulePath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "aetherpak.yaml")
	// Default submodule path is manifests/<reponame>; place a manifest there.
	subPath := filepath.Join(dir, "manifests/myrepo")
	if err := os.MkdirAll(subPath, 0755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, subPath)

	err := Run(Options{
		ConfigPath: cfgPath,
		Source:     SourceGit,
		GitURL:     "https://example.com/myrepo.git",
		Git:        gitutil.NewWithExecutor(executil.NewMockExecutor()),
		Confirm:    true,
		WorkDir:    dir,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	s := string(mustRead(t, cfgPath))
	if !strings.Contains(s, "manifest: manifests/myrepo/org.example.App.yaml") {
		t.Errorf("default submodule path not manifests/<reponame>:\n%s", s)
	}
}

func TestArchForGOARCH(t *testing.T) {
	if got := archForGOARCH("arm64"); got != "aarch64" {
		t.Errorf("arm64 -> %q, want aarch64", got)
	}
	for _, g := range []string{"amd64", "386", "riscv64", ""} {
		if got := archForGOARCH(g); got != "x86_64" {
			t.Errorf("%q -> %q, want x86_64", g, got)
		}
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestRunBundleFingerprints(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "aetherpak.yaml")

	err := Run(Options{
		ConfigPath: cfgPath,
		Source:     SourceBundle,
		BundleURL:  "https://example.com/app.flatpak",
		ID:         "org.example.App",
		Confirm:    true,
		Fetch: func(url string, _ ProgressFunc) (string, string, error) {
			return "", strings.Repeat("a", 64), nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, _ := os.ReadFile(cfgPath)
	s := string(data)
	if !strings.Contains(s, "bundles:") || !strings.Contains(s, strings.Repeat("a", 64)) {
		t.Errorf("config missing bundle:\n%s", s)
	}
}

func TestRunBundleRejectsMultipleArches(t *testing.T) {
	dir := t.TempDir()
	called := false
	err := Run(Options{
		ConfigPath: filepath.Join(dir, "aetherpak.yaml"),
		Source:     SourceBundle,
		BundleURL:  "https://example.com/app.flatpak",
		ID:         "org.example.App",
		Arches:     []string{"x86_64", "aarch64"},
		Confirm:    true,
		Fetch: func(string, ProgressFunc) (string, string, error) {
			called = true
			return "", "", nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "single architecture") {
		t.Fatalf("expected single-architecture error, got: %v", err)
	}
	if called {
		t.Error("must reject before downloading the bundle")
	}
}

func TestRunGitRejectsTraversalSubmodulePath(t *testing.T) {
	dir := t.TempDir()
	mock := executil.NewMockExecutor()
	err := Run(Options{
		ConfigPath:    filepath.Join(dir, "aetherpak.yaml"),
		Source:        SourceGit,
		GitURL:        "https://example.com/repo.git",
		SubmodulePath: "../../evil",
		Git:           gitutil.NewWithExecutor(mock),
		Confirm:       true,
		WorkDir:       dir,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid submodule path") {
		t.Fatalf("expected invalid submodule path error, got: %v", err)
	}
	if len(mock.Commands) != 0 {
		t.Errorf("no git commands should run for an invalid path, got %v", mock.Commands)
	}
}

func TestRunBundleChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "aetherpak.yaml")
	err := Run(Options{
		ConfigPath:   cfgPath,
		Source:       SourceBundle,
		BundleURL:    "https://example.com/app.flatpak",
		ID:           "org.example.App",
		BundleSHA256: strings.Repeat("b", 64),
		Confirm:      true,
		Fetch: func(url string, _ ProgressFunc) (string, string, error) {
			return "", strings.Repeat("a", 64), nil
		},
	})
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		t.Error("config should not be written on checksum mismatch")
	}
}

func TestRunBundleRemovesTempFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "aetherpak.yaml")
	tmp := filepath.Join(dir, "downloaded.flatpak")
	if err := os.WriteFile(tmp, []byte("bundle"), 0644); err != nil {
		t.Fatal(err)
	}
	err := Run(Options{
		ConfigPath: cfgPath,
		Source:     SourceBundle,
		BundleURL:  "https://example.com/app.flatpak",
		ID:         "org.example.App",
		Confirm:    true,
		Fetch: func(url string, _ ProgressFunc) (string, string, error) {
			return tmp, strings.Repeat("a", 64), nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, statErr := os.Stat(tmp); !os.IsNotExist(statErr) {
		t.Errorf("temp bundle file was not removed: %v", statErr)
	}
}

func TestRunGitAcceptStoresManifestPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "aetherpak.yaml")
	subPath := filepath.Join(dir, "sources/org.example.App")
	if err := os.MkdirAll(subPath, 0755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, subPath)

	mock := executil.NewMockExecutor()
	err := Run(Options{
		ConfigPath:    cfgPath,
		Source:        SourceGit,
		GitURL:        "https://example.com/repo.git",
		SubmodulePath: "sources/org.example.App",
		Git:           gitutil.NewWithExecutor(mock),
		Confirm:       true, // accept
		WorkDir:       dir,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(data), "manifest: sources/org.example.App/org.example.App.yaml") {
		t.Errorf("git manifest path not stored relative to config dir:\n%s", data)
	}
}

func TestRunGitRollbackOnDecline(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "aetherpak.yaml")
	// Simulate a checked-out submodule containing a manifest.
	subPath := filepath.Join(dir, "sources/org.example.App")
	if err := os.MkdirAll(subPath, 0755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, subPath)

	mock := executil.NewMockExecutor()
	g := gitutil.NewWithExecutor(mock)

	err := Run(Options{
		ConfigPath:    cfgPath,
		Source:        SourceGit,
		GitURL:        "https://example.com/repo.git",
		SubmodulePath: "sources/org.example.App",
		Git:           g,
		Confirm:       false,
		In:            strings.NewReader("n\n"), // decline
		Out:           &strings.Builder{},
		WorkDir:       dir,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		t.Error("config should not be written on decline")
	}
	// Expect submodule add + recursive init, then the removal steps on decline.
	var names [][]string
	for _, c := range mock.Commands {
		names = append(names, c.Args)
	}
	if len(names) < 5 {
		t.Fatalf("expected add+init+removal commands, got %v", names)
	}
	// The removal sequence begins with submodule deinit.
	if names[2][0] != "-c" || names[2][1] != "safe.directory=*" || names[2][2] != "submodule" || names[2][3] != "deinit" {
		t.Errorf("rollback sequence not invoked: %v", names)
	}
}

func TestRunDuplicateIDNonInteractive(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeManifest(t, dir)
	cfgPath := filepath.Join(dir, "aetherpak.yaml")
	if err := os.WriteFile(cfgPath, []byte("apps:\n  - id: org.example.App\n    manifest: x.yaml\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := Run(Options{
		ConfigPath:   cfgPath,
		Source:       SourceManifest,
		ManifestPath: manifestPath,
		Confirm:      true,
	})
	if err == nil {
		t.Fatal("expected duplicate-id error in non-interactive mode")
	}
}

func TestRunDeclineEOF(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeManifest(t, dir)
	cfgPath := filepath.Join(dir, "aetherpak.yaml")

	err := Run(Options{
		ConfigPath:   cfgPath,
		Source:       SourceManifest,
		ManifestPath: manifestPath,
		Confirm:      false,
		In:           strings.NewReader(""), // EOF immediately
		Out:          &strings.Builder{},
	})
	if err == nil {
		t.Fatal("expected error on stdin EOF when Confirm is false, got nil")
	}
	if !strings.Contains(err.Error(), "non-interactive environment detected") {
		t.Errorf("expected non-interactive env error, got: %v", err)
	}
}
