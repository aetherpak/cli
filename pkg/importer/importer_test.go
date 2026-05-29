package importer

import (
	"os"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
)

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
