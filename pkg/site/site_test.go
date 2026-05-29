package site

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppTitle(t *testing.T) {
	tests := []struct {
		name       string
		appdataXML string
		appID      string
		expected   string
	}{
		{
			name:       "empty xml fallback to suffix",
			appdataXML: "",
			appID:      "org.example.cool-app",
			expected:   "cool-app",
		},
		{
			name:       "empty xml fallback to plain appid when no dots",
			appdataXML: "",
			appID:      "cool-app",
			expected:   "cool-app",
		},
		{
			name: "standard name extract",
			appdataXML: `<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop">
  <id>org.example.cool-app</id>
  <name>My Cool App</name>
</component>`,
			appID:    "org.example.cool-app",
			expected: "My Cool App",
		},
		{
			name: "name extract with languages",
			appdataXML: `<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop">
  <id>org.example.cool-app</id>
  <name xml:lang="fr">Mon App Cool</name>
  <name>My Cool App</name>
</component>`,
			appID:    "org.example.cool-app",
			expected: "My Cool App",
		},
		{
			name: "name extract fallback to first lang when no default",
			appdataXML: `<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop">
  <id>org.example.cool-app</id>
  <name xml:lang="de">German App</name>
  <name xml:lang="fr">French App</name>
</component>`,
			appID:    "org.example.cool-app",
			expected: "German App",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := appTitle(tt.appdataXML, tt.appID)
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestBuildSiteCustomTemplate(t *testing.T) {
	tempDir := t.TempDir()
	templateFile := filepath.Join(tempDir, "custom.html")
	templateContent := "Hello Custom __AETHERPAK_REPO_TITLE__!"
	if err := os.WriteFile(templateFile, []byte(templateContent), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	opts := SiteOptions{
		SiteDir:       tempDir,
		LandingPage:   true,
		IndexTemplate: templateFile,
		RepoTitle:     "TitleTest",
	}

	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed: %v", err)
	}

	indexPath := filepath.Join(tempDir, "index.html")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	expected := "Hello Custom TitleTest!"
	if string(data) != expected {
		t.Errorf("expected %q, got %q", expected, string(data))
	}
}
