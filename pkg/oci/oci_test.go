package oci

import (
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
	"github.com/google/go-containerregistry/pkg/registry"
)

func setupMockPushExecutor() executil.Executor {
	mockExec := executil.NewMockExecutor()
	mockExec.OnCommand = func(cmd *executil.MockCommand) {
		if cmd.Name == "flatpak" && len(cmd.Args) > 0 && cmd.Args[0] == "build-bundle" {
			cmd.RunFunc = func() error {
				var ociDir string
				if len(cmd.Args) > 4 {
					ociDir = cmd.Args[4]
				}
				if ociDir == "" {
					return nil
				}
				blobsDir := filepath.Join(ociDir, "blobs", "sha256")
				if err := os.MkdirAll(blobsDir, 0755); err != nil {
					return err
				}
				writeBlob := func(content []byte) (string, error) {
					h := sha256.Sum256(content)
					digest := fmt.Sprintf("%x", h)
					if err := os.WriteFile(filepath.Join(blobsDir, digest), content, 0644); err != nil {
						return "", err
					}
					return digest, nil
				}
				configBlob := []byte(`{"config":{"Labels":{"org.flatpak.ref":"app/org.example.App/x86_64/stable"}}}`)
				configDigest, err := writeBlob(configBlob)
				if err != nil {
					return err
				}
				manifestBlob := []byte(fmt.Sprintf(`{
					"schemaVersion": 2,
					"mediaType": "application/vnd.oci.image.manifest.v1+json",
					"config": {
						"mediaType": "application/vnd.oci.image.config.v1+json",
						"digest": "sha256:%s",
						"size": %d
					},
					"layers": []
				}`, configDigest, len(configBlob)))
				manifestDigest, err := writeBlob(manifestBlob)
				if err != nil {
					return err
				}
				indexJSON := fmt.Sprintf(`{
					"schemaVersion": 2,
					"manifests": [
						{
							"mediaType": "application/vnd.oci.image.manifest.v1+json",
							"digest": "sha256:%s",
							"size": %d
						}
					]
				}`, manifestDigest, len(manifestBlob))
				return os.WriteFile(filepath.Join(ociDir, "index.json"), []byte(indexJSON), 0644)
			}
		}
	}
	return mockExec
}

func TestPush(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()

	regHost := strings.TrimPrefix(regServer.URL, "http://")

	recordsDir, err := os.MkdirTemp("", "aetherpak-test-records-*")
	if err != nil {
		t.Fatalf("failed to create temp records dir: %v", err)
	}
	defer os.RemoveAll(recordsDir)

	mockExec := setupMockPushExecutor()

	opts := PushOptions{
		AppID:         "org.example.App",
		Arch:          "x86_64",
		Branch:        "stable",
		Registry:      regHost,
		OCIRepository: "org.example.app",
		RepoPath:      "repo",
		RecordsDir:    recordsDir,
		Insecure:      true,
		Executor:      mockExec,
		AllowUnsigned: true, // required for unsigned push to succeed
	}

	res, err := Push(opts)
	if err != nil {
		t.Fatalf("expected push to succeed, got %v", err)
	}

	if !strings.HasPrefix(res.Digest, "sha256:") {
		t.Errorf("unexpected digest format: %s", res.Digest)
	}

	var bundleRan bool
	for _, cmd := range mockExec.(*executil.MockExecutor).Commands {
		if cmd.Name == "flatpak" && len(cmd.Args) > 0 && cmd.Args[0] == "build-bundle" {
			bundleRan = true
		}
	}
	if !bundleRan {
		t.Errorf("expected flatpak build-bundle to have run")
	}
}

func TestPushUnsignedFailsByDefault(t *testing.T) {
	recordsDir := t.TempDir()
	mockExec := setupMockPushExecutor()

	opts := PushOptions{
		AppID:         "org.example.App",
		Arch:          "x86_64",
		Branch:        "stable",
		Registry:      "localhost:5000",
		OCIRepository: "org.example.app",
		RepoPath:      "repo",
		RecordsDir:    recordsDir,
		Insecure:      true,
		Executor:      mockExec,
		AllowUnsigned: false, // default
	}

	_, err := Push(opts)
	if err == nil {
		t.Fatalf("expected error when GPG keys are missing and unsigned is not allowed")
	}
	if !strings.Contains(err.Error(), "GPG signing keys are missing") {
		t.Errorf("expected missing keys error, got: %v", err)
	}
}

func TestPushNoSignSucceedsUnsigned(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()

	regHost := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()
	mockExec := setupMockPushExecutor()

	opts := PushOptions{
		AppID:         "org.example.App",
		Arch:          "x86_64",
		Branch:        "stable",
		Registry:      regHost,
		OCIRepository: "org.example.app",
		RepoPath:      "repo",
		RecordsDir:    recordsDir,
		Insecure:      true,
		Executor:      mockExec,
		NoSign:        true,
		AllowUnsigned: false, // no-sign mode bypasses allow-unsigned check
	}

	_, err := Push(opts)
	if err != nil {
		t.Fatalf("expected push to succeed when no-sign is enabled, got %v", err)
	}
}
