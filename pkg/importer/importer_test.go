package importer

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
	"github.com/aetherpak/aetherpak/pkg/repoinfo"
)

func TestFetchDownloadsAndHashes(t *testing.T) {
	payload := []byte("hello-bundle")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	var lastDownloaded, lastTotal int64
	path, sum, err := Fetch(srv.URL, func(downloaded, total int64) {
		lastDownloaded = downloaded
		lastTotal = total
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer os.Remove(path)

	want := fmt.Sprintf("%x", sha256.Sum256(payload))
	if sum != want {
		t.Errorf("sha256 = %s, want %s", sum, want)
	}
	if lastTotal != int64(len(payload)) {
		t.Errorf("progress total = %d, want %d", lastTotal, len(payload))
	}
	if lastDownloaded != int64(len(payload)) {
		t.Errorf("progress downloaded = %d, want %d", lastDownloaded, len(payload))
	}
	data, _ := os.ReadFile(path)
	if string(data) != string(payload) {
		t.Errorf("downloaded content mismatch")
	}
}

func TestImport(t *testing.T) {
	// Create dummy bundle file
	tmpFile, err := os.CreateTemp("", "dummy-bundle-*.flatpak")
	if err != nil {
		t.Fatalf("failed to create temp bundle file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte("dummy content")); err != nil {
		t.Fatalf("failed to write to temp bundle file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp bundle file: %v", err)
	}

	destRepo, err := os.MkdirTemp("", "aetherpak-test-dest-repo-*")
	if err != nil {
		t.Fatalf("failed to create temp dest repo: %v", err)
	}
	defer os.RemoveAll(destRepo)

	mockExec := executil.NewMockExecutor()

	// ostree refs command should return a valid ref
	mockExec.OutMap["ostree"] = []byte("app/org.example.App/x86_64/stable\n")

	opts := ImportOptions{
		AppID:      "org.example.App",
		Arch:       "x86_64",
		Branch:     "stable",
		BundlePath: tmpFile.Name(),
		RepoPath:   destRepo,
		Executor:   mockExec,
	}

	err = Import(opts)
	if err != nil {
		t.Fatalf("expected import to succeed, got %v", err)
	}

	// Verify the commands executed
	var initScratchRan, importBundleRan, refsRan, initDestRan, rebindRan bool
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "ostree" {
			argsJoined := strings.Join(cmd.Args, " ")
			if strings.Contains(argsJoined, "init") {
				if strings.Contains(argsJoined, destRepo) {
					initDestRan = true
				} else {
					initScratchRan = true
				}
			} else if strings.Contains(argsJoined, "refs") {
				refsRan = true
			}
		} else if cmd.Name == "flatpak" {
			argsJoined := strings.Join(cmd.Args, " ")
			if strings.Contains(argsJoined, "build-import-bundle") {
				importBundleRan = true
			} else if strings.Contains(argsJoined, "build-commit-from") {
				rebindRan = true
			}
		}
	}

	if !initScratchRan {
		t.Errorf("expected ostree scratch init to have run")
	}
	if !importBundleRan {
		t.Errorf("expected flatpak build-import-bundle to have run")
	}
	if !refsRan {
		t.Errorf("expected ostree refs to have run")
	}
	if !initDestRan {
		t.Errorf("expected ostree dest init to have run")
	}
	if !rebindRan {
		t.Errorf("expected flatpak build-commit-from to have run")
	}
}

func TestImportDownload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("mock bundle content"))
	}))
	defer ts.Close()

	destRepo, err := os.MkdirTemp("", "aetherpak-test-dest-repo-*")
	if err != nil {
		t.Fatalf("failed to create temp dest repo: %v", err)
	}
	defer os.RemoveAll(destRepo)

	mockExec := executil.NewMockExecutor()
	mockExec.OutMap["ostree"] = []byte("app/org.example.App/x86_64/stable\n")

	opts := ImportOptions{
		AppID:     "org.example.App",
		Arch:      "x86_64",
		Branch:    "stable",
		BundleURL: ts.URL,
		RepoPath:  destRepo,
		Executor:  mockExec,
	}

	err = Import(opts)
	if err != nil {
		t.Fatalf("expected import to succeed, got %v", err)
	}

	var importBundleRan bool
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak" {
			argsJoined := strings.Join(cmd.Args, " ")
			if strings.Contains(argsJoined, "build-import-bundle") {
				importBundleRan = true
			}
		}
	}
	if !importBundleRan {
		t.Errorf("expected flatpak build-import-bundle to have run")
	}
}

func TestImportDownloadExceedsLimit(t *testing.T) {
	oldMax := maxBundleSize
	maxBundleSize = 10
	defer func() { maxBundleSize = oldMax }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this response is definitely longer than 10 bytes"))
	}))
	defer ts.Close()

	destRepo := t.TempDir()
	mockExec := executil.NewMockExecutor()

	opts := ImportOptions{
		AppID:     "org.example.App",
		Arch:      "x86_64",
		Branch:    "stable",
		BundleURL: ts.URL,
		RepoPath:  destRepo,
		Executor:  mockExec,
	}

	err := Import(opts)
	if err == nil {
		t.Fatalf("expected error when download exceeds max bundle size")
	}
	if !strings.Contains(err.Error(), "exceeded maximum size limit") {
		t.Errorf("expected exceeded limit error, got: %v", err)
	}
}

func TestRebindRefs(t *testing.T) {
	destRepo, err := os.MkdirTemp("", "aetherpak-test-rebind-dest-*")
	if err != nil {
		t.Fatalf("failed to create temp dest repo: %v", err)
	}
	defer os.RemoveAll(destRepo)

	mockExec := executil.NewMockExecutor()

	refs := []repoinfo.Info{
		{
			AppID:   "org.example.App1",
			Arch:    "x86_64",
			Branch:  "stable",
			RefType: "app",
		},
		{
			AppID:   "org.example.App2",
			Arch:    "aarch64",
			Branch:  "beta",
			RefType: "app",
		},
	}

	opts := RebindRefsOptions{
		SrcRepo:  "/tmp/src-repo",
		DestRepo: destRepo,
		Refs:     refs,
		Executor: mockExec,
	}

	err = RebindRefs(opts)
	if err != nil {
		t.Fatalf("expected RebindRefs to succeed, got %v", err)
	}

	// Verify the commands executed
	var initDestRan bool
	var rebindApp1Ran, rebindApp2Ran bool

	for _, cmd := range mockExec.Commands {
		if cmd.Name == "ostree" {
			argsJoined := strings.Join(cmd.Args, " ")
			if strings.Contains(argsJoined, "init") && strings.Contains(argsJoined, destRepo) {
				initDestRan = true
			}
		} else if cmd.Name == "flatpak" {
			argsJoined := strings.Join(cmd.Args, " ")
			if strings.Contains(argsJoined, "build-commit-from") {
				if strings.Contains(argsJoined, "org.example.App1") && strings.Contains(argsJoined, "x86_64") && strings.Contains(argsJoined, "stable") {
					rebindApp1Ran = true
				}
				if strings.Contains(argsJoined, "org.example.App2") && strings.Contains(argsJoined, "aarch64") && strings.Contains(argsJoined, "beta") {
					rebindApp2Ran = true
				}
			}
		}
	}

	if !initDestRan {
		t.Errorf("expected ostree target init command to run")
	}
	if !rebindApp1Ran {
		t.Errorf("expected flatpak rebind for App1 to run")
	}
	if !rebindApp2Ran {
		t.Errorf("expected flatpak rebind for App2 to run")
	}
}

func TestImportRuntimeBundle(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "dummy-bundle-*.flatpak")
	if err != nil {
		t.Fatalf("failed to create temp bundle file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte("dummy content")); err != nil {
		t.Fatalf("failed to write to temp bundle file: %v", err)
	}
	tmpFile.Close()

	destRepo, err := os.MkdirTemp("", "aetherpak-test-dest-repo-*")
	if err != nil {
		t.Fatalf("failed to create temp dest repo: %v", err)
	}
	defer os.RemoveAll(destRepo)

	mockExec := executil.NewMockExecutor()

	// ostree refs returns a runtime ref instead of app ref
	mockExec.OutMap["ostree"] = []byte("runtime/org.freedesktop.Sdk.Extension.xrt/x86_64/stable\n")

	opts := ImportOptions{
		BundlePath: tmpFile.Name(),
		RepoPath:   destRepo,
		Executor:   mockExec,
	}

	err = Import(opts)
	if err != nil {
		t.Fatalf("expected import of runtime bundle to succeed, got %v", err)
	}

	// Verify rebind used runtime/ prefix
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak" {
			argsJoined := strings.Join(cmd.Args, " ")
			if strings.Contains(argsJoined, "build-commit-from") {
				if !strings.Contains(argsJoined, "runtime/org.freedesktop.Sdk.Extension.xrt/x86_64/stable") {
					t.Errorf("expected rebind to use runtime/ ref prefix, got args: %s", argsJoined)
				}
			}
		}
	}
}

func TestRebindRefsRuntime(t *testing.T) {
	destRepo, err := os.MkdirTemp("", "aetherpak-test-rebind-runtime-*")
	if err != nil {
		t.Fatalf("failed to create temp dest repo: %v", err)
	}
	defer os.RemoveAll(destRepo)

	mockExec := executil.NewMockExecutor()

	refs := []repoinfo.Info{
		{
			AppID:   "org.freedesktop.Sdk.Extension.xrt",
			Arch:    "x86_64",
			Branch:  "stable",
			RefType: "runtime",
		},
	}

	opts := RebindRefsOptions{
		SrcRepo:  "/tmp/src-repo",
		DestRepo: destRepo,
		Refs:     refs,
		Executor: mockExec,
	}

	err = RebindRefs(opts)
	if err != nil {
		t.Fatalf("expected RebindRefs to succeed, got %v", err)
	}

	// Verify runtime/ prefix used in the rebind command
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak" {
			argsJoined := strings.Join(cmd.Args, " ")
			if strings.Contains(argsJoined, "build-commit-from") {
				if !strings.Contains(argsJoined, "runtime/org.freedesktop.Sdk.Extension.xrt/x86_64/stable") {
					t.Errorf("expected rebind to use runtime/ ref, got args: %s", argsJoined)
				}
			}
		}
	}
}
