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

func TestPush(t *testing.T) {
	// Create an in-memory registry test server
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()

	// Resolve the registry host from the server URL (remove http://)
	regHost := strings.TrimPrefix(regServer.URL, "http://")

	// Create temporary records directory
	recordsDir, err := os.MkdirTemp("", "aetherpak-test-records-*")
	if err != nil {
		t.Fatalf("failed to create temp records dir: %v", err)
	}
	defer os.RemoveAll(recordsDir)

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

				// Helper to write blob and return its SHA-256 digest
				writeBlob := func(content []byte) (string, error) {
					h := sha256.Sum256(content)
					digest := fmt.Sprintf("%x", h)
					if err := os.WriteFile(filepath.Join(blobsDir, digest), content, 0644); err != nil {
						return "", err
					}
					return digest, nil
				}

				// 1. Write config blob
				configBlob := []byte(`{"config":{"Labels":{"org.flatpak.ref":"app/org.example.App/x86_64/stable"}}}`)
				configDigest, err := writeBlob(configBlob)
				if err != nil {
					return err
				}

				// 2. Write manifest blob referencing config digest
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

				// 3. Write index.json referencing manifest digest
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

				if err := os.WriteFile(filepath.Join(ociDir, "index.json"), []byte(indexJSON), 0644); err != nil {
					return err
				}

				return nil
			}
		}
	}

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
	}

	res, err := Push(opts)
	if err != nil {
		t.Fatalf("expected push to succeed, got %v", err)
	}

	if !strings.HasPrefix(res.Digest, "sha256:") {
		t.Errorf("unexpected digest format: %s", res.Digest)
	}

	// Check if the mock flatpak build-bundle was run
	var bundleRan bool
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak" && len(cmd.Args) > 0 && cmd.Args[0] == "build-bundle" {
			bundleRan = true
		}
	}
	if !bundleRan {
		t.Errorf("expected flatpak build-bundle to have run")
	}
}
