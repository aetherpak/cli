package site

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePreviewDummySingleGPG(t *testing.T) {
	tempDir := t.TempDir()

	opts := PreviewOptions{
		SiteDir:    tempDir,
		Live:       false,
		GPG:        true,
		Apps:       "single",
		RemoteName: "preview-remote",
		RepoTitle:  "Preview Single App",
	}

	if err := GeneratePreview(opts); err != nil {
		t.Fatalf("GeneratePreview failed: %v", err)
	}

	// 1. Assert index.html exists and contains the title
	indexPath := filepath.Join(tempDir, "index.html")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	indexHTML := string(indexBytes)
	if !strings.Contains(indexHTML, "Preview Single App") {
		t.Errorf("expected index.html to contain repo title, got: %s", indexHTML)
	}

	// 2. Assert index/static has exactly 1 app (org.gnome.Builder)
	staticPath := filepath.Join(tempDir, "index", "static")
	staticBytes, err := os.ReadFile(staticPath)
	if err != nil {
		t.Fatalf("failed to read index/static: %v", err)
	}
	var flatpakIndex FlatpakIndex
	if err := json.Unmarshal(staticBytes, &flatpakIndex); err != nil {
		t.Fatalf("failed to unmarshal index/static JSON: %v", err)
	}

	// Verify the flatpak repo only has org.gnome.Builder images
	var foundBuilder bool
	for _, pkg := range flatpakIndex.Results {
		if pkg.Name == "gnome/builder" {
			foundBuilder = true
		}
		for _, img := range pkg.Images {
			refVal := img.Labels["org.flatpak.ref"]
			if !strings.Contains(refVal, "org.gnome.Builder") {
				t.Errorf("expected only org.gnome.Builder in single app mode, but found: %s", refVal)
			}
		}
	}
	if !foundBuilder {
		t.Error("expected to find gnome/builder package in index results")
	}

	// 3. Assert sigs/signing.json is enabled
	signingPath := filepath.Join(tempDir, "sigs", "signing.json")
	signingBytes, err := os.ReadFile(signingPath)
	if err != nil {
		t.Fatalf("failed to read signing.json: %v", err)
	}
	var signingData map[string]interface{}
	if err := json.Unmarshal(signingBytes, &signingData); err != nil {
		t.Fatalf("failed to unmarshal signing.json: %v", err)
	}
	if enabled, ok := signingData["enabled"].(bool); !ok || !enabled {
		t.Errorf("expected GPG signing to be enabled in signing.json, got: %v", signingData)
	}

	// 4. Assert sigs/key.asc exists
	keyPath := filepath.Join(tempDir, "sigs", "key.asc")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("expected key.asc to exist since GPG is enabled")
	}
}

func TestGeneratePreviewDummyMultipleNoGPG(t *testing.T) {
	tempDir := t.TempDir()

	opts := PreviewOptions{
		SiteDir:    tempDir,
		Live:       false,
		GPG:        false,
		Apps:       "multiple",
		RemoteName: "preview-remote",
		RepoTitle:  "Preview Multiple Apps No GPG",
	}

	if err := GeneratePreview(opts); err != nil {
		t.Fatalf("GeneratePreview failed: %v", err)
	}

	// 1. Assert index/static has all 3 apps
	staticPath := filepath.Join(tempDir, "index", "static")
	staticBytes, err := os.ReadFile(staticPath)
	if err != nil {
		t.Fatalf("failed to read index/static: %v", err)
	}
	var flatpakIndex FlatpakIndex
	if err := json.Unmarshal(staticBytes, &flatpakIndex); err != nil {
		t.Fatalf("failed to unmarshal index/static JSON: %v", err)
	}

	appsFound := make(map[string]bool)
	for _, pkg := range flatpakIndex.Results {
		for _, img := range pkg.Images {
			refVal := img.Labels["org.flatpak.ref"]
			parts := strings.Split(refVal, "/")
			if len(parts) >= 2 {
				appsFound[parts[1]] = true
			}
		}
	}

	for _, expectedApp := range []string{"org.gnome.Builder", "com.obsproject.Studio", "org.videolan.VLC"} {
		if !appsFound[expectedApp] {
			t.Errorf("expected app %s to be present in multiple apps preview", expectedApp)
		}
	}

	// 2. Assert sigs/signing.json is disabled
	signingPath := filepath.Join(tempDir, "sigs", "signing.json")
	signingBytes, err := os.ReadFile(signingPath)
	if err != nil {
		t.Fatalf("failed to read signing.json: %v", err)
	}
	var signingData map[string]interface{}
	if err := json.Unmarshal(signingBytes, &signingData); err != nil {
		t.Fatalf("failed to unmarshal signing.json: %v", err)
	}
	if enabled, ok := signingData["enabled"].(bool); !ok || enabled {
		t.Errorf("expected GPG signing to be disabled in signing.json, got: %v", signingData)
	}

	// 3. Assert sigs/key.asc does not exist
	keyPath := filepath.Join(tempDir, "sigs", "key.asc")
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Error("expected key.asc NOT to exist since GPG is disabled")
	}
}

func TestGeneratePreviewLive(t *testing.T) {
	// Setup mock live index server
	mockIndex := FlatpakIndex{
		Registry: "ghcr.io",
		Results: []IndexResultPackage{
			{
				Name: "my/live-app",
				Images: []IndexImage{
					{
						Digest:       "sha256:7777777777777777777777777777777777777777777777777777777777777777",
						MediaType:    "application/vnd.oci.image.manifest.v1+json",
						OS:           "linux",
						Architecture: "amd64",
						Tags:         []string{"stable"},
						Labels: map[string]string{
							"org.flatpak.ref":      "app/org.live.App/x86_64/stable",
							"org.flatpak.metadata": "[Application]\nname=org.live.App",
						},
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/index/static") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(mockIndex)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tempDir := t.TempDir()

	opts := PreviewOptions{
		SiteDir:      tempDir,
		Live:         true,
		LiveURL:      server.URL,
		GPG:          false,
		RemoteName:   "live-preview-remote",
		RepoTitle:    "Live Preview",
		RepoHomepage: "https://homepage.local",
	}

	if err := GeneratePreview(opts); err != nil {
		t.Fatalf("GeneratePreview failed: %v", err)
	}

	// Assert index/static has been fetched and written correctly
	staticPath := filepath.Join(tempDir, "index", "static")
	staticBytes, err := os.ReadFile(staticPath)
	if err != nil {
		t.Fatalf("failed to read index/static: %v", err)
	}
	var flatpakIndex FlatpakIndex
	if err := json.Unmarshal(staticBytes, &flatpakIndex); err != nil {
		t.Fatalf("failed to unmarshal index/static JSON: %v", err)
	}

	if len(flatpakIndex.Results) != 1 || flatpakIndex.Results[0].Name != "my/live-app" {
		t.Errorf("expected fetched index to match mock, got: %+v", flatpakIndex)
	}
}
