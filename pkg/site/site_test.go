package site

import (
	"os"
	"path/filepath"
	"strings"
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
		AllowUnsigned: true, // required for unsigned repository generation to succeed
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

func TestBuildSiteUnsignedFailsByDefault(t *testing.T) {
	tempDir := t.TempDir()
	opts := SiteOptions{
		SiteDir:       tempDir,
		AllowUnsigned: false, // default
	}

	err := BuildSite(opts)
	if err == nil {
		t.Fatalf("expected error when GPG keys are missing and unsigned repository is not allowed")
	}
	if !strings.Contains(err.Error(), "GPG signing keys are missing") {
		t.Errorf("expected missing keys error, got: %v", err)
	}
}

func TestBuildSiteNoSignSucceedsUnsigned(t *testing.T) {
	tempDir := t.TempDir()
	opts := SiteOptions{
		SiteDir:       tempDir,
		NoSign:        true,
		AllowUnsigned: false, // no-sign mode bypasses allow-unsigned check
	}

	err := BuildSite(opts)
	if err != nil {
		t.Fatalf("expected BuildSite to succeed when no-sign is enabled, got %v", err)
	}
}

func TestBuildSiteEscapesLogoURL(t *testing.T) {
	tempDir := t.TempDir()
	templateFile := filepath.Join(tempDir, "custom.html")
	templateContent := "Branding: __AETHERPAK_BRANDING_LOGO_HTML__"
	if err := os.WriteFile(templateFile, []byte(templateContent), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	opts := SiteOptions{
		SiteDir:       tempDir,
		LandingPage:   true,
		IndexTemplate: templateFile,
		LogoURL:       `https://example.com/logo.png" onerror="alert(1)`,
		AllowUnsigned: true,
	}

	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed: %v", err)
	}

	indexPath := filepath.Join(tempDir, "index.html")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	expectedEscapedURL := `https://example.com/logo.png&#34; onerror=&#34;alert(1)`
	if !strings.Contains(string(data), expectedEscapedURL) {
		t.Errorf("expected escaped URL in output, got: %s", string(data))
	}
	if strings.Contains(string(data), `onerror="alert(1)"`) {
		t.Errorf("found unescaped attributes in output: %s", string(data))
	}
}

func TestSanitizeINIValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Normal Value",
			expected: "Normal Value",
		},
		{
			input:    "Value\nWith\nNewlines",
			expected: "ValueWithNewlines",
		},
		{
			input:    "Value\r\nWith\r\nWindows\r\nNewlines",
			expected: "ValueWithWindowsNewlines",
		},
		{
			input:    "Value=With=Equals",
			expected: "Value=With=Equals",
		},
	}

	for _, tt := range tests {
		actual := sanitizeINIValue(tt.input)
		if actual != tt.expected {
			t.Errorf("sanitizeINIValue(%q) = %q; expected %q", tt.input, actual, tt.expected)
		}
	}
}
