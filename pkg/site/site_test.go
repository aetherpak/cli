package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/record"
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

func TestBuildSiteStructuredTemplate(t *testing.T) {
	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// Set up mock records using record.WriteRecord
	rec1 := record.Record{
		AppID:    "org.example.app1",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app1",
		Registry: "ghcr.io",
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}
	labels1 := map[string]string{
		"org.flatpak.ref":                   "app/org.example.app1/x86_64/stable",
		"org.flatpak.metadata":              "[Application]\nname=org.example.app1",
		"org.flatpak.timestamp":             "1717200000", // June 1, 2024 00:00:00 UTC
		"org.flatpak.installed-size":        "20971520",   // 20 MB
		"org.flatpak.download-size":         "5242880",    // 5 MB
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>Example App One</name><summary>This is example app one</summary></component>`,
		"org.freedesktop.appstream.icon-64": "https://example.com/icon.png",
	}
	if _, err := record.WriteRecord(recordsDir, rec1, labels1); err != nil {
		t.Fatalf("failed to write record 1: %v", err)
	}

	rec2 := record.Record{
		AppID:    "org.example.app1",
		Arch:     "aarch64",
		Branch:   "stable",
		Name:     "example/app1",
		Registry: "ghcr.io",
		Digest:   "sha256:2222222222222222222222222222222222222222222222222222222222222222",
	}
	labels2 := map[string]string{
		"org.flatpak.ref":                   "app/org.example.app1/aarch64/stable",
		"org.flatpak.metadata":              "[Application]\nname=org.example.app1",
		"org.flatpak.timestamp":             "1717200000",
		"org.flatpak.installed-size":        "20971520",
		"org.flatpak.download-size":         "5242880",
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>Example App One</name><summary>This is example app one</summary></component>`,
		"org.freedesktop.appstream.icon-64": "https://example.com/icon.png",
	}
	if _, err := record.WriteRecord(recordsDir, rec2, labels2); err != nil {
		t.Fatalf("failed to write record 2: %v", err)
	}

	// Set up mock record with edge cases (malformed timestamp, missing sizes, XML XSS injection)
	rec3 := record.Record{
		AppID:    "org.example.app2",
		Arch:     "x86_64",
		Branch:   "beta",
		Name:     "example/app2",
		Registry: "ghcr.io",
		Digest:   "sha256:3333333333333333333333333333333333333333333333333333333333333333",
	}
	labels3 := map[string]string{
		"org.flatpak.ref":                   "app/org.example.app2/x86_64/beta",
		"org.flatpak.metadata":              "[Application]\nname=org.example.app2",
		"org.flatpak.timestamp":             "not-a-number",
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>Example App Two &lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;</name><summary>Summary with &lt; XSS</summary></component>`,
		"org.freedesktop.appstream.icon-64": "https://example.com/icon2.png",
	}
	if _, err := record.WriteRecord(recordsDir, rec3, labels3); err != nil {
		t.Fatalf("failed to write record 3: %v", err)
	}

	templateFile := filepath.Join(tempDir, "custom.html")
	templateContent := `{{range .Apps}}
App: {{.Name}} ({{.ID}}) - {{.Summary}} - {{.Icon}}
{{range .Branches}}
- Branch: {{.Branch}}
  Date: {{.FormattedDate}}
  HelperDate: {{formatDate .Timestamp "2006-01-02"}}
  Arches: {{join .Arches "/"}}
  InstalledSize: {{formatSize .InstalledSize}}
  DownloadSize: {{formatSize .DownloadSize}}
  InstallCommand: {{.InstallCmd}}
  RefFile: {{.RefFile}}
{{end}}
{{end}}`
	if err := os.WriteFile(templateFile, []byte(templateContent), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	opts := SiteOptions{
		RecordsDir:    recordsDir,
		SiteDir:       siteDir,
		LandingPage:   true,
		IndexTemplate: templateFile,
		RepoTitle:     "TitleTest",
		RemoteName:    "myremote",
		AllowUnsigned: true,
	}

	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed: %v", err)
	}

	indexPath := filepath.Join(siteDir, "index.html")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	output := string(data)
	t.Logf("Generated output:\n%s", output)

	expectedAppLine := "App: Example App One (org.example.app1) - This is example app one - https://example.com/icon.png"
	if !strings.Contains(output, expectedAppLine) {
		t.Errorf("expected output to contain %q", expectedAppLine)
	}

	expectedBranchLine := "- Branch: stable"
	if !strings.Contains(output, expectedBranchLine) {
		t.Errorf("expected output to contain %q", expectedBranchLine)
	}

	expectedArchesLine := "Arches: aarch64/x86_64" // alphabetical
	if !strings.Contains(output, expectedArchesLine) {
		t.Errorf("expected output to contain %q", expectedArchesLine)
	}

	expectedDateLine := "Date: Jun 01, 2024"
	if !strings.Contains(output, expectedDateLine) {
		t.Errorf("expected output to contain %q", expectedDateLine)
	}

	expectedHelperDateLine := "HelperDate: 2024-06-01"
	if !strings.Contains(output, expectedHelperDateLine) {
		t.Errorf("expected output to contain %q", expectedHelperDateLine)
	}

	expectedInstalledSize := "InstalledSize: 20 MB"
	if !strings.Contains(output, expectedInstalledSize) {
		t.Errorf("expected output to contain %q", expectedInstalledSize)
	}

	expectedDownloadSize := "DownloadSize: 5.0 MB"
	if !strings.Contains(output, expectedDownloadSize) {
		t.Errorf("expected output to contain %q", expectedDownloadSize)
	}

	expectedInstallCmd := "InstallCommand: flatpak install --user myremote org.example.app1//stable"
	if !strings.Contains(output, expectedInstallCmd) {
		t.Errorf("expected output to contain %q", expectedInstallCmd)
	}

	expectedRefFile := "RefFile: refs/org.example.app1-stable.flatpakref"
	if !strings.Contains(output, expectedRefFile) {
		t.Errorf("expected output to contain %q", expectedRefFile)
	}

	// Edge case assertions for org.example.app2 (malformed timestamp/missing sizes/HTML injection safety)
	// 1. HTML escaping: html/template should safely escape the script tag in App 2 name
	expectedApp2EscapedName := "Example App Two &lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"
	if !strings.Contains(output, expectedApp2EscapedName) {
		t.Errorf("expected output to contain HTML-escaped script tag: %q", expectedApp2EscapedName)
	}

	// 2. Malformed timestamp: should result in empty date fields and HelperDate being empty
	expectedApp2Date := "Date: "
	if !strings.Contains(output, expectedApp2Date) {
		t.Errorf("expected output to contain empty Date field for malformed timestamp: %q", expectedApp2Date)
	}
	expectedApp2HelperDate := "HelperDate: "
	if !strings.Contains(output, expectedApp2HelperDate) {
		t.Errorf("expected output to contain empty HelperDate for malformed timestamp: %q", expectedApp2HelperDate)
	}

	// 3. Missing sizes: should render as "0 B"
	expectedApp2Sizes := "InstalledSize: 0 B\n  DownloadSize: 0 B"
	if !strings.Contains(output, expectedApp2Sizes) {
		t.Errorf("expected output to contain 0 B for missing size fields: %q", expectedApp2Sizes)
	}
}
