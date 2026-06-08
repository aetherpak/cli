package oci

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
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

func TestCleanTag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"stable", "stable"},
		{"v1.2.3", "v1_2_3"},
		{"app-branch", "app-branch"},
		{"feature/branch", "feature_branch"},
		{"dirty:tag@name", "dirty_tag_name"},
	}

	for _, tt := range tests {
		actual := CleanTag(tt.input)
		if actual != tt.expected {
			t.Errorf("CleanTag(%q) = %q; expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestPushSigned(t *testing.T) {
	// Generate GPG Key
	entity, err := openpgp.NewEntity("AetherPak Test", "Test key", "test@aetherpak.local", nil)
	if err != nil {
		t.Fatalf("failed to generate key entity: %v", err)
	}

	var privKeyBlock bytes.Buffer
	wPriv, err := armor.Encode(&privKeyBlock, openpgp.PrivateKeyType, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := entity.SerializePrivate(wPriv, nil); err != nil {
		t.Fatal(err)
	}
	wPriv.Close()

	// Start mock registry
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
		GPGKeys:       []string{privKeyBlock.String()},
	}

	res, err := Push(opts)
	if err != nil {
		t.Fatalf("expected push to succeed when signed, got %v", err)
	}

	// Verify signature output files are created in the records cell dir
	if res.CellDir == "" {
		t.Fatal("expected CellDir to be returned in PushResult")
	}

	digestHex := strings.TrimPrefix(res.Digest, "sha256:")
	sigFile := filepath.Join(res.CellDir, "sigs", fmt.Sprintf("org.example.app@sha256=%s", digestHex), "signature-1")
	if _, err := os.Stat(sigFile); os.IsNotExist(err) {
		t.Errorf("expected signature file to exist at %s, but got not found", sigFile)
	}
}

func TestPushRuntimeRefType(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()

	regHost := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()
	mockExec := setupMockPushExecutor()

	opts := PushOptions{
		AppID:         "org.freedesktop.Sdk.Extension.xrt",
		Arch:          "x86_64",
		Branch:        "stable",
		Registry:      regHost,
		OCIRepository: "org.freedesktop.sdk.extension.xrt",
		RepoPath:      "repo",
		RecordsDir:    recordsDir,
		Insecure:      true,
		Executor:      mockExec,
		AllowUnsigned: true,
		RefType:       "runtime",
	}

	res, err := Push(opts)
	if err != nil {
		t.Fatalf("expected push to succeed with runtime ref type, got %v", err)
	}

	if res.CellDir == "" {
		t.Fatal("expected CellDir to be set")
	}

	// Verify the record.json contains runtime/ ref prefix
	recPath := filepath.Join(res.CellDir, "record.json")
	data, err := os.ReadFile(recPath)
	if err != nil {
		t.Fatalf("failed to read record.json: %v", err)
	}

	if !strings.Contains(string(data), "runtime/org.freedesktop.Sdk.Extension.xrt/x86_64/stable") {
		t.Errorf("expected record to contain runtime/ ref, got: %s", string(data))
	}
}
