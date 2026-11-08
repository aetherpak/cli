package site

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/logger"
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

	// Set up runtime record
	recRuntime := record.Record{
		AppID:    "org.freedesktop.Sdk.Extension.xrt",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/sdk-ext",
		Registry: "ghcr.io",
		Digest:   "sha256:4444444444444444444444444444444444444444444444444444444444444444",
		Ref:      "runtime/org.freedesktop.Sdk.Extension.xrt/x86_64/stable",
	}
	labelsRuntime := map[string]string{
		"org.flatpak.ref":                   "runtime/org.freedesktop.Sdk.Extension.xrt/x86_64/stable",
		"org.flatpak.metadata":              "[Extension]\nname=org.freedesktop.Sdk.Extension.xrt",
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>XRT Extension</name><summary>This is XRT extension</summary></component>`,
	}
	if _, err := record.WriteRecord(recordsDir, recRuntime, labelsRuntime); err != nil {
		t.Fatalf("failed to write runtime record: %v", err)
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

	expectedRuntimeLine := "App: XRT Extension (org.freedesktop.Sdk.Extension.xrt) - This is XRT extension - "
	if !strings.Contains(output, expectedRuntimeLine) {
		t.Errorf("expected output to contain %q", expectedRuntimeLine)
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

func TestBackfillSignaturesValidation(t *testing.T) {
	// Setup a mock HTTP server to handle signature downloads
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "trigger-500") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.HasSuffix(r.URL.Path, "signature-1") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("mock-signature-content"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	opts := SiteOptions{
		SiteDir:  tempDir,
		PagesURL: server.URL,
	}

	tests := []struct {
		name         string
		pkgName      string
		digest       string
		pagesURL     string
		expectError  bool
		expectFile   string // Relative path of file we expect to be created, if any
		expectNoFile string // Path we expect NOT to be created
	}{
		{
			name:        "valid sha256 backfill",
			pkgName:     "my/valid-package",
			digest:      "sha256:d577273ff885c3f84da8b3c859c4050d25271d596c3f3f05d527ff250567f812",
			expectError: false,
			expectFile:  "sigs/my/valid-package@sha256=d577273ff885c3f84da8b3c859c4050d25271d596c3f3f05d527ff250567f812/signature-1",
		},
		{
			name:        "valid sha512 backfill",
			pkgName:     "my/another-valid-package",
			digest:      "sha512:c6827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827",
			expectError: false,
			expectFile:  "sigs/my/another-valid-package@sha512=c6827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827827/signature-1",
		},
		{
			name:         "invalid algorithm sha1 is skipped",
			pkgName:      "my/invalid-algo",
			digest:       "sha1:d577273ff885c3f84da8b3c859c4050d25271d596c3f",
			expectError:  false,
			expectNoFile: "sigs/my/invalid-algo@sha1=d577273ff885c3f84da8b3c859c4050d25271d596c3f/signature-1",
		},
		{
			name:         "invalid digest non-hex is skipped",
			pkgName:      "my/invalid-digest",
			digest:       "sha256:not-a-hex-value-12345",
			expectError:  false,
			expectNoFile: "sigs/my/invalid-digest@sha256=not-a-hex-value-12345/signature-1",
		},
		{
			name:         "traversal package name starts with .. is skipped",
			pkgName:      "../../unsafe-package",
			digest:       "sha256:d577273ff885c3f84da8b3c859c4050d25271d596c3f3f05d527ff250567f812",
			expectError:  false,
			expectNoFile: "sigs/../../unsafe-package@sha256=d577273ff885c3f84da8b3c859c4050d25271d596c3f3f05d527ff250567f812/signature-1",
		},
		{
			name:         "traversal package name absolute is skipped",
			pkgName:      "/etc/unsafe-package",
			digest:       "sha256:d577273ff885c3f84da8b3c859c4050d25271d596c3f3f05d527ff250567f812",
			expectError:  false,
			expectNoFile: "sigs/etc/unsafe-package@sha256=d577273ff885c3f84da8b3c859c4050d25271d596c3f3f05d527ff250567f812/signature-1",
		},
		{
			name:         "network error is skipped gracefully",
			pkgName:      "my/unreachable-package",
			digest:       "sha256:d577273ff885c3f84da8b3c859c4050d25271d596c3f3f05d527ff250567f812",
			pagesURL:     "http://127.0.0.1:65530", // highly likely unreachable
			expectError:  false,
			expectNoFile: "sigs/my/unreachable-package@sha256=d577273ff885c3f84da8b3c859c4050d25271d596c3f3f05d527ff250567f812/signature-1",
		},
		{
			name:         "server 500 error is skipped gracefully",
			pkgName:      "trigger-500",
			digest:       "sha256:d577273ff885c3f84da8b3c859c4050d25271d596c3f3f05d527ff250567f812",
			expectError:  false,
			expectNoFile: "sigs/trigger-500@sha256=d577273ff885c3f84da8b3c859c4050d25271d596c3f3f05d527ff250567f812/signature-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Construct a minimal FlatpakIndex containing our test cases
			index := FlatpakIndex{
				Results: []IndexResultPackage{
					{
						Name: tt.pkgName,
						Images: []IndexImage{
							{
								Digest: tt.digest,
								Labels: map[string]string{
									"org.flatpak.metadata": "[Application]\nname=test",
								},
							},
						},
					},
				},
			}

			testOpts := opts
			if tt.pagesURL != "" {
				testOpts.PagesURL = tt.pagesURL
			}

			err := backfillSignatures(testOpts, index, "sigs")
			if (err != nil) != tt.expectError {
				t.Fatalf("backfillSignatures returned error %v, expected error: %v", err, tt.expectError)
			}

			if tt.expectFile != "" {
				fullPath := filepath.Join(tempDir, tt.expectFile)
				if _, err := os.Stat(fullPath); os.IsNotExist(err) {
					t.Errorf("expected signature file to exist at %s, but it was not found", fullPath)
				} else {
					content, _ := os.ReadFile(fullPath)
					if string(content) != "mock-signature-content" {
						t.Errorf("unexpected content in backfilled signature file: %s", string(content))
					}
				}
			}

			if tt.expectNoFile != "" {
				fullPath := filepath.Join(tempDir, tt.expectNoFile)
				if _, err := os.Stat(fullPath); err == nil {
					t.Errorf("expected signature file NOT to exist at %s, but it was found", fullPath)
				}
			}
		})
	}
}

func TestIsUnderDir(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		dir      string
		path     string
		expected bool
	}{
		{
			dir:      tempDir,
			path:     filepath.Join(tempDir, "sigs/somepkg"),
			expected: true,
		},
		{
			dir:      tempDir,
			path:     filepath.Join(tempDir, "../outside"),
			expected: false,
		},
		{
			dir:      tempDir,
			path:     "/absolute/path/that/is/outside",
			expected: false,
		},
	}

	for _, tt := range tests {
		result, err := isUnderDir(tt.dir, tt.path)
		if err != nil {
			t.Errorf("isUnderDir(%q, %q) returned unexpected error: %v", tt.dir, tt.path, err)
		}
		if result != tt.expected {
			t.Errorf("isUnderDir(%q, %q) = %v; expected %v", tt.dir, tt.path, result, tt.expected)
		}
	}
}

func TestCopyFile(t *testing.T) {
	tempDir := t.TempDir()
	src := filepath.Join(tempDir, "src.txt")
	dst := filepath.Join(tempDir, "dst.txt")

	content := []byte("hello copyFile")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	// Test successful copy
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("copied content mismatch: got %q, expected %q", string(got), string(content))
	}

	// Test copy from non-existent file
	if err := copyFile(filepath.Join(tempDir, "nonexistent"), dst); err == nil {
		t.Error("expected error copying from non-existent file")
	}

	// Test copy to unwritable location
	if err := copyFile(src, filepath.Join(tempDir, "nonexistent-dir/dst.txt")); err == nil {
		t.Error("expected error copying to unwritable location")
	}
}

func TestMapArch(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"x86_64", "amd64"},
		{"X86_64", "amd64"},
		{"aarch64", "arm64"},
		{"AARCH64", "arm64"},
		{"i386", "386"},
		{"i586", "386"},
		{"i686", "386"},
		{"arm", "arm"},
		{"armv7hl", "arm"},
		{"other", "other"},
		{"OTHER", "other"},
	}

	for _, tt := range tests {
		actual := mapArch(tt.input)
		if actual != tt.expected {
			t.Errorf("mapArch(%q) = %q; expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestBuildSiteReconcile(t *testing.T) {
	// Setup a mock HTTP registry
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// App 1 exists
		if strings.Contains(r.URL.Path, "sha256:1111111111111111111111111111111111111111111111111111111111111111") {
			w.WriteHeader(http.StatusOK)
			return
		}
		// App 2 does not exist (404)
		if strings.Contains(r.URL.Path, "sha256:2222222222222222222222222222222222222222222222222222222222222222") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// Set up mock records:
	// App 1 (valid/exists in registry)
	rec1 := record.Record{
		AppID:    "org.example.app1",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app1",
		Registry: server.URL,
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}
	labels1 := map[string]string{
		"org.flatpak.ref":      "app/org.example.app1/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app1",
	}
	if _, err := record.WriteRecord(recordsDir, rec1, labels1); err != nil {
		t.Fatalf("failed to write record 1: %v", err)
	}

	// App 2 (missing from registry - should be removed during reconcile!)
	rec2 := record.Record{
		AppID:    "org.example.app2",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app2",
		Registry: server.URL,
		Digest:   "sha256:2222222222222222222222222222222222222222222222222222222222222222",
	}
	labels2 := map[string]string{
		"org.flatpak.ref":      "app/org.example.app2/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app2",
	}
	if _, err := record.WriteRecord(recordsDir, rec2, labels2); err != nil {
		t.Fatalf("failed to write record 2: %v", err)
	}

	opts := SiteOptions{
		RecordsDir:    recordsDir,
		SiteDir:       siteDir,
		Reconcile:     true,
		Insecure:      true,
		AllowUnsigned: true,
	}

	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed with reconcile=true: %v", err)
	}

	// Verify that index/static file was written and only contains app1, NOT app2!
	indexPath := filepath.Join(siteDir, "index", "static")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index static: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "org.example.app1") {
		t.Error("expected index to contain app1")
	}
	if strings.Contains(content, "org.example.app2") {
		t.Error("expected index to NOT contain app2 since it failed registry reconciliation")
	}
}

func TestBuildSiteRuntimeRef(t *testing.T) {
	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// Set up a runtime extension record
	rec := record.Record{
		AppID:    "org.freedesktop.Sdk.Extension.xrt",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/sdk-ext",
		Registry: "ghcr.io",
		Digest:   "sha256:4444444444444444444444444444444444444444444444444444444444444444",
		Ref:      "runtime/org.freedesktop.Sdk.Extension.xrt/x86_64/stable",
	}
	labels := map[string]string{
		"org.flatpak.ref":      "runtime/org.freedesktop.Sdk.Extension.xrt/x86_64/stable",
		"org.flatpak.metadata": "[Extension]\nname=org.freedesktop.Sdk.Extension.xrt",
	}
	if _, err := record.WriteRecord(recordsDir, rec, labels); err != nil {
		t.Fatalf("failed to write runtime record: %v", err)
	}

	opts := SiteOptions{
		RecordsDir:    recordsDir,
		SiteDir:       siteDir,
		AllowUnsigned: true,
		RemoteName:    "myremote",
	}

	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed: %v", err)
	}

	// Verify index/static contains the runtime ref
	indexPath := filepath.Join(siteDir, "index", "static")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index static: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "org.freedesktop.Sdk.Extension.xrt") {
		t.Error("expected index to contain runtime extension")
	}
	if !strings.Contains(content, "runtime/org.freedesktop.Sdk.Extension.xrt/x86_64/stable") {
		t.Error("expected index to contain runtime/ ref in org.flatpak.ref label")
	}

	// Verify flatpakref file has IsRuntime=true
	refsDir := filepath.Join(siteDir, "refs")
	refFile := filepath.Join(refsDir, "org.freedesktop.Sdk.Extension.xrt-stable.flatpakref")
	refData, err := os.ReadFile(refFile)
	if err != nil {
		t.Fatalf("failed to read flatpakref file: %v", err)
	}
	refContent := string(refData)
	if !strings.Contains(refContent, "IsRuntime=true") {
		t.Errorf("expected flatpakref to contain IsRuntime=true, got:\n%s", refContent)
	}
}

func TestWriteFlatpakRepoAndRefWithLookaside(t *testing.T) {
	tempDir := t.TempDir()
	opts := SiteOptions{
		PagesURL:   "https://pages.example.com",
		RemoteName: "myremote",
		RepoTitle:  "My Repo",
	}

	// 1. Test writeFlatpakRepoFile
	err := writeFlatpakRepoFile(tempDir, "ghcr.io", "some-gpg-key-base64", opts)
	if err != nil {
		t.Fatalf("writeFlatpakRepoFile failed: %v", err)
	}

	repoFilePath := filepath.Join(tempDir, "myremote.flatpakrepo")
	repoBytes, err := os.ReadFile(repoFilePath)
	if err != nil {
		t.Fatalf("failed to read generated flatpakrepo file: %v", err)
	}
	repoContent := string(repoBytes)

	if !strings.Contains(repoContent, "GPGKey=some-gpg-key-base64") {
		t.Errorf("expected flatpakrepo to contain GPGKey, got:\n%s", repoContent)
	}
	if !strings.Contains(repoContent, "SignatureLookaside=https://pages.example.com/sigs") {
		t.Errorf("expected flatpakrepo to contain SignatureLookaside, got:\n%s", repoContent)
	}

	// 2. Test writeFlatpakRefs
	index := FlatpakIndex{
		Registry: "ghcr.io",
		Results: []IndexResultPackage{
			{
				Name: "example/app",
				Images: []IndexImage{
					{
						Labels: map[string]string{
							"org.flatpak.ref": "app/org.example.app/x86_64/stable",
						},
					},
				},
			},
		},
	}

	sigLookasideURL := "https://pages.example.com/sigs"
	err = writeFlatpakRefs(tempDir, index, "some-gpg-key-base64", sigLookasideURL, opts)
	if err != nil {
		t.Fatalf("writeFlatpakRefs failed: %v", err)
	}

	refFilePath := filepath.Join(tempDir, "refs", "org.example.app-stable.flatpakref")
	refBytes, err := os.ReadFile(refFilePath)
	if err != nil {
		t.Fatalf("failed to read generated flatpakref file: %v", err)
	}
	refContent := string(refBytes)

	if !strings.Contains(refContent, "GPGKey=some-gpg-key-base64") {
		t.Errorf("expected flatpakref to contain GPGKey, got:\n%s", refContent)
	}
	if !strings.Contains(refContent, "SignatureLookaside=https://pages.example.com/sigs") {
		t.Errorf("expected flatpakref to contain SignatureLookaside, got:\n%s", refContent)
	}
}

func TestBuildSiteReconcileActiveApps(t *testing.T) {
	// Setup a mock HTTP registry that returns 200 OK for everything (both apps exist in registry)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// Set up mock records:
	// App 1 (configured as active)
	rec1 := record.Record{
		AppID:    "org.example.app1",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app1",
		Registry: server.URL,
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}
	labels1 := map[string]string{
		"org.flatpak.ref":      "app/org.example.app1/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app1\n[Extension org.example.app1.Plugin]\ndirectory=extensions/plugin",
	}
	if _, err := record.WriteRecord(recordsDir, rec1, labels1); err != nil {
		t.Fatalf("failed to write record 1: %v", err)
	}

	// App 2 (NOT configured as active, but exists in registry)
	rec2 := record.Record{
		AppID:    "org.example.app2",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app2",
		Registry: server.URL,
		Digest:   "sha256:2222222222222222222222222222222222222222222222222222222222222222",
	}
	labels2 := map[string]string{
		"org.flatpak.ref":      "app/org.example.app2/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app2\n[Extension org.example.app2.Plugin]\ndirectory=extensions/plugin",
	}
	if _, err := record.WriteRecord(recordsDir, rec2, labels2); err != nil {
		t.Fatalf("failed to write record 2: %v", err)
	}

	// Debug, Locale and Sources companions of App 1. Their parent is active,
	// so they must survive reconcile even though their own app ids are not
	// configured.
	companions := []struct {
		appID  string
		digest string
		ref    string
	}{
		{"org.example.app1.Debug", "sha256:3333333333333333333333333333333333333333333333333333333333333333", "runtime/org.example.app1.Debug/x86_64/stable"},
		{"org.example.app1.Locale", "sha256:4444444444444444444444444444444444444444444444444444444444444444", "runtime/org.example.app1.Locale/x86_64/stable"},
		{"org.example.app1.Sources", "sha256:7777777777777777777777777777777777777777777777777777777777777777", "runtime/org.example.app1.Sources/x86_64/stable"},
	}
	for _, companion := range companions {
		rec := record.Record{
			AppID:    companion.appID,
			Arch:     "x86_64",
			Branch:   "stable",
			Name:     "example/app1",
			Registry: server.URL,
			Digest:   companion.digest,
		}
		labels := map[string]string{
			"org.flatpak.ref":      companion.ref,
			"org.flatpak.metadata": "[Runtime]\nname=" + companion.appID + "\nparent=org.example.app1/x86_64/stable",
		}
		if _, err := record.WriteRecord(recordsDir, rec, labels); err != nil {
			t.Fatalf("failed to write companion record %s: %v", companion.appID, err)
		}
	}

	// Debug companion of App 2, which is not configured.
	rec2Debug := record.Record{
		AppID:    "org.example.app2.Debug",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app2",
		Registry: server.URL,
		Digest:   "sha256:5555555555555555555555555555555555555555555555555555555555555555",
	}
	labels2Debug := map[string]string{
		"org.flatpak.ref":      "runtime/org.example.app2.Debug/x86_64/stable",
		"org.flatpak.metadata": "[Runtime]\nname=org.example.app2.Debug\nparent=org.example.app2/x86_64/stable",
	}
	if _, err := record.WriteRecord(recordsDir, rec2Debug, labels2Debug); err != nil {
		t.Fatalf("failed to write app2 debug record: %v", err)
	}

	// App id sharing App 1's prefix but unrelated to it. Matching must not keep
	// it just because the active id is a prefix.
	rec1Extra := record.Record{
		AppID:    "org.example.app1extra",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app1extra",
		Registry: server.URL,
		Digest:   "sha256:6666666666666666666666666666666666666666666666666666666666666666",
	}
	labels1Extra := map[string]string{
		"org.flatpak.ref":      "app/org.example.app1extra/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app1extra",
	}
	if _, err := record.WriteRecord(recordsDir, rec1Extra, labels1Extra); err != nil {
		t.Fatalf("failed to write lookalike app record: %v", err)
	}

	// Extension refs declared in App 1's metadata, plus the locale of one of
	// them. An extension of App 2 is not declared by an active app.
	extensions := []struct {
		appID  string
		name   string
		ref    string
		digest string
	}{
		{"org.example.app1.Plugin", "example/app1", "runtime/org.example.app1.Plugin/x86_64/stable", "sha256:" + strings.Repeat("8", 64)},
		{"org.example.app1.Plugin.Locale", "example/app1", "runtime/org.example.app1.Plugin.Locale/x86_64/stable", "sha256:" + strings.Repeat("9", 64)},
		{"org.example.app2.Plugin", "example/app2", "runtime/org.example.app2.Plugin/x86_64/stable", "sha256:" + strings.Repeat("a", 64)},
	}
	for _, extension := range extensions {
		rec := record.Record{
			AppID:    extension.appID,
			Arch:     "x86_64",
			Branch:   "stable",
			Name:     extension.name,
			Registry: server.URL,
			Digest:   extension.digest,
		}
		labels := map[string]string{
			"org.flatpak.ref":      extension.ref,
			"org.flatpak.metadata": "[Runtime]\nname=" + extension.appID,
		}
		if _, err := record.WriteRecord(recordsDir, rec, labels); err != nil {
			t.Fatalf("failed to write extension record %s: %v", extension.appID, err)
		}
	}

	opts := SiteOptions{
		RecordsDir:    recordsDir,
		SiteDir:       siteDir,
		Reconcile:     true,
		ActiveAppIDs:  []string{"org.example.app1"}, // Only App 1 is active!
		Insecure:      true,
		AllowUnsigned: true,
	}

	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed with reconcile=true and ActiveAppIDs: %v", err)
	}

	// Verify that index/static holds App 1 and its companions, and nothing
	// belonging to an unconfigured app.
	indexPath := filepath.Join(siteDir, "index", "static")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index static: %v", err)
	}
	var index FlatpakIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("failed to parse index static: %v", err)
	}
	var refs []string
	for _, pkg := range index.Results {
		for _, img := range pkg.Images {
			refs = append(refs, img.Labels["org.flatpak.ref"])
		}
	}
	sort.Strings(refs)
	want := []string{
		"app/org.example.app1/x86_64/stable",
		"runtime/org.example.app1.Debug/x86_64/stable",
		"runtime/org.example.app1.Locale/x86_64/stable",
		"runtime/org.example.app1.Plugin.Locale/x86_64/stable",
		"runtime/org.example.app1.Plugin/x86_64/stable",
		"runtime/org.example.app1.Sources/x86_64/stable",
	}
	sort.Strings(want)
	if !slices.Equal(refs, want) {
		t.Errorf("index refs = %v, want %v", refs, want)
	}
}

func TestBuildSiteReconcileSeededIndexKeepsExtensions(t *testing.T) {
	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records") // absent: reconcile-only run
	siteDir := filepath.Join(tempDir, "site")

	seeded := FlatpakIndex{
		Results: []IndexResultPackage{{
			Name: "example/app",
			Images: []IndexImage{
				{
					Digest:       "sha256:" + strings.Repeat("1", 64),
					Architecture: "amd64",
					Labels: map[string]string{
						"org.flatpak.ref":      "app/org.example.app/x86_64/stable",
						"org.flatpak.metadata": "[Application]\nname=org.example.app\n[Extension org.example.app.Plugin]\ndirectory=extensions/plugin",
					},
				},
				{
					Digest:       "sha256:" + strings.Repeat("2", 64),
					Architecture: "amd64",
					Labels: map[string]string{
						"org.flatpak.ref":      "runtime/org.example.app.Plugin/x86_64/stable",
						"org.flatpak.metadata": "[Runtime]\nname=org.example.app.Plugin",
					},
				},
				{
					Digest:       "sha256:" + strings.Repeat("3", 64),
					Architecture: "amd64",
					Labels: map[string]string{
						"org.flatpak.ref":      "runtime/org.example.app.Plugin.Locale/x86_64/stable",
						"org.flatpak.metadata": "[Runtime]\nname=org.example.app.Plugin.Locale",
					},
				},
			},
		}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/index/static":
			_ = json.NewEncoder(w).Encode(seeded)
		case strings.HasPrefix(r.URL.Path, "/sigs/"):
			// No signature lookaside in this fixture.
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()
	seeded.Registry = server.URL

	opts := SiteOptions{
		PagesURL:      server.URL,
		RecordsDir:    recordsDir,
		SiteDir:       siteDir,
		Reconcile:     true,
		ActiveAppIDs:  []string{"org.example.app"},
		Insecure:      true,
		AllowUnsigned: true,
	}
	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed for a seeded index: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(siteDir, "index", "static"))
	if err != nil {
		t.Fatalf("failed to read index static: %v", err)
	}
	var index FlatpakIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("failed to parse index static: %v", err)
	}
	var refs []string
	for _, pkg := range index.Results {
		for _, img := range pkg.Images {
			refs = append(refs, img.Labels["org.flatpak.ref"])
		}
	}
	sort.Strings(refs)
	want := []string{
		"app/org.example.app/x86_64/stable",
		"runtime/org.example.app.Plugin.Locale/x86_64/stable",
		"runtime/org.example.app.Plugin/x86_64/stable",
	}
	if !slices.Equal(refs, want) {
		t.Errorf("index refs = %v, want %v", refs, want)
	}
}

func TestBuildSiteSeedsIndexFromLocalFile(t *testing.T) {
	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	seeded := FlatpakIndex{
		Registry: "ghcr.io",
		Results: []IndexResultPackage{
			{
				Name: "example/app",
				Images: []IndexImage{{
					Digest:       "sha256:" + strings.Repeat("a", 64),
					MediaType:    "application/vnd.oci.image.manifest.v1+json",
					OS:           "linux",
					Architecture: "amd64",
					Tags:         []string{"stable"},
					Labels: map[string]string{
						"org.flatpak.ref":      "app/org.example.app/x86_64/stable",
						"org.flatpak.metadata": "[Application]\nname=org.example.app",
					},
				}},
			},
			{
				Name: "example/other",
				Images: []IndexImage{{
					Digest:       "sha256:" + strings.Repeat("b", 64),
					MediaType:    "application/vnd.oci.image.manifest.v1+json",
					OS:           "linux",
					Architecture: "amd64",
					Tags:         []string{"nightly"},
					Labels: map[string]string{
						"org.flatpak.ref":      "app/org.example.other/x86_64/nightly",
						"org.flatpak.metadata": "[Application]\nname=org.example.other",
					},
				}},
			},
		},
	}
	seedPath := filepath.Join(tempDir, "seed", "index", "static")
	if err := os.MkdirAll(filepath.Dir(seedPath), 0755); err != nil {
		t.Fatalf("failed to create seed index directory: %v", err)
	}
	seedData, err := json.Marshal(seeded)
	if err != nil {
		t.Fatalf("failed to marshal seed index: %v", err)
	}
	if err := os.WriteFile(seedPath, seedData, 0644); err != nil {
		t.Fatalf("failed to write seed index: %v", err)
	}

	// This build only has a record for the stable channel of example/app.
	rec := record.Record{
		AppID:    "org.example.app",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app",
		Registry: "ghcr.io",
		Digest:   "sha256:" + strings.Repeat("c", 64),
		Ref:      "app/org.example.app/x86_64/stable",
	}
	labels := map[string]string{
		"org.flatpak.ref":      "app/org.example.app/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app",
	}
	if _, err := record.WriteRecord(recordsDir, rec, labels); err != nil {
		t.Fatalf("failed to write record: %v", err)
	}

	// The Pages index must not be consulted when a local seed is given.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index/static" {
			t.Error("index/static must not be fetched when --index-file is set")
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	opts := SiteOptions{
		PagesURL:      server.URL,
		IndexFile:     seedPath,
		RecordsDir:    recordsDir,
		SiteDir:       siteDir,
		AllowUnsigned: true,
	}
	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed with a local seed index: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(siteDir, "index", "static"))
	if err != nil {
		t.Fatalf("failed to read index static: %v", err)
	}
	var index FlatpakIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("failed to parse index static: %v", err)
	}

	digests := make(map[string]string)
	var refs []string
	for _, pkg := range index.Results {
		for _, img := range pkg.Images {
			ref := img.Labels["org.flatpak.ref"]
			refs = append(refs, ref)
			digests[ref] = img.Digest
		}
	}
	sort.Strings(refs)
	want := []string{
		"app/org.example.app/x86_64/stable",
		"app/org.example.other/x86_64/nightly",
	}
	if !slices.Equal(refs, want) {
		t.Errorf("index refs = %v, want %v", refs, want)
	}
	if got := digests["app/org.example.app/x86_64/stable"]; got != rec.Digest {
		t.Errorf("stable ref digest = %q, want the current record digest %q", got, rec.Digest)
	}
}

func TestBuildSiteIndexFileErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		write   bool
		want    string
	}{
		{name: "missing file", want: "failed to read seed index"},
		{name: "malformed json", content: "{not-json", write: true, want: "failed to parse seed index"},
		{name: "missing results", content: `{"Registry":"ghcr.io"}`, write: true, want: "failed to parse seed index"},
		{name: "null results", content: `{"Registry":"ghcr.io","Results":null}`, write: true, want: "failed to parse seed index"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			seedPath := filepath.Join(tempDir, "index", "static")
			if tt.write {
				if err := os.MkdirAll(filepath.Dir(seedPath), 0755); err != nil {
					t.Fatalf("failed to create seed directory: %v", err)
				}
				if err := os.WriteFile(seedPath, []byte(tt.content), 0644); err != nil {
					t.Fatalf("failed to write seed index: %v", err)
				}
			}

			err := BuildSite(SiteOptions{
				IndexFile:     seedPath,
				RecordsDir:    filepath.Join(tempDir, "records"),
				SiteDir:       filepath.Join(tempDir, "site"),
				AllowUnsigned: true,
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("BuildSite error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestBuildSiteSeedsIndexAndSignaturesFromSeedDir(t *testing.T) {
	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")
	seedDir := filepath.Join(tempDir, "seed")

	stableDigest := "sha256:" + strings.Repeat("a", 64)
	recordDigest := "sha256:" + strings.Repeat("c", 64)
	otherDigest := "sha256:" + strings.Repeat("b", 64)
	recordSigRel := filepath.Join("example", "app@sha256="+strings.Repeat("c", 64), "signature-1")
	otherSigRel := filepath.Join("example", "other@sha256="+strings.Repeat("b", 64), "signature-1")

	seeded := FlatpakIndex{
		Registry: "ghcr.io",
		Results: []IndexResultPackage{
			{
				Name: "example/app",
				Images: []IndexImage{{
					Digest:       stableDigest,
					Architecture: "amd64",
					Labels: map[string]string{
						"org.flatpak.ref":      "app/org.example.app/x86_64/stable",
						"org.flatpak.metadata": "[Application]\nname=org.example.app",
					},
				}},
			},
			{
				Name: "example/other",
				Images: []IndexImage{{
					Digest:       otherDigest,
					Architecture: "amd64",
					Labels: map[string]string{
						"org.flatpak.ref":      "app/org.example.other/x86_64/nightly",
						"org.flatpak.metadata": "[Application]\nname=org.example.other",
					},
				}},
			},
		},
	}
	seedPath := filepath.Join(seedDir, "index", "static")
	if err := os.MkdirAll(filepath.Dir(seedPath), 0755); err != nil {
		t.Fatalf("failed to create seed index directory: %v", err)
	}
	seedData, err := json.Marshal(seeded)
	if err != nil {
		t.Fatalf("failed to marshal seed index: %v", err)
	}
	if err := os.WriteFile(seedPath, seedData, 0644); err != nil {
		t.Fatalf("failed to write seed index: %v", err)
	}

	// Seed signatures include the other channel and a stale signature for the
	// digest this build is about to replace.
	seedSigs := map[string]string{
		otherSigRel:    "other-signature",
		recordSigRel:   "stale-seed-signature",
		"key.asc":      "old-public-key",
		"signing.json": `{"enabled":true}`,
	}
	for rel, content := range seedSigs {
		path := filepath.Join(seedDir, "sigs", rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create seed signature directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write seed signature: %v", err)
		}
	}

	rec := record.Record{
		AppID:    "org.example.app",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app",
		Registry: "ghcr.io",
		Digest:   recordDigest,
		Ref:      "app/org.example.app/x86_64/stable",
	}
	labels := map[string]string{
		"org.flatpak.ref":      "app/org.example.app/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app",
	}
	cellDir, err := record.WriteRecord(recordsDir, rec, labels)
	if err != nil {
		t.Fatalf("failed to write record: %v", err)
	}

	// The current record signature must win over the seeded one.
	recordSig := filepath.Join(cellDir, "sigs", recordSigRel)
	if err := os.MkdirAll(filepath.Dir(recordSig), 0755); err != nil {
		t.Fatalf("failed to create record signature directory: %v", err)
	}
	if err := os.WriteFile(recordSig, []byte("record-signature"), 0644); err != nil {
		t.Fatalf("failed to write record signature: %v", err)
	}

	// A seeded site directory must not touch Pages for index or signatures.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected Pages request for %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	opts := SiteOptions{
		PagesURL:      server.URL,
		SeedDir:       seedDir,
		RecordsDir:    recordsDir,
		SiteDir:       siteDir,
		AllowUnsigned: true,
	}
	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed with a seed directory: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(siteDir, "index", "static"))
	if err != nil {
		t.Fatalf("failed to read index static: %v", err)
	}
	var index FlatpakIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("failed to parse index static: %v", err)
	}
	var refs []string
	for _, pkg := range index.Results {
		for _, img := range pkg.Images {
			refs = append(refs, img.Labels["org.flatpak.ref"])
		}
	}
	sort.Strings(refs)
	wantRefs := []string{
		"app/org.example.app/x86_64/stable",
		"app/org.example.other/x86_64/nightly",
	}
	if !slices.Equal(refs, wantRefs) {
		t.Errorf("index refs = %v, want %v", refs, wantRefs)
	}

	for rel, want := range map[string]string{
		otherSigRel:  "other-signature",
		recordSigRel: "record-signature",
	} {
		got, err := os.ReadFile(filepath.Join(siteDir, "sigs", rel))
		if err != nil {
			t.Fatalf("failed to read signature %s: %v", rel, err)
		}
		if string(got) != want {
			t.Errorf("signature %s = %q, want %q", rel, got, want)
		}
	}

	// Lookaside metadata from the seed must not leak into an unsigned rebuild.
	if _, err := os.Stat(filepath.Join(siteDir, "sigs", "key.asc")); !os.IsNotExist(err) {
		t.Errorf("seeded key.asc was copied into the output: %v", err)
	}
	signingJSON, err := os.ReadFile(filepath.Join(siteDir, "sigs", "signing.json"))
	if err != nil {
		t.Fatalf("failed to read signing.json: %v", err)
	}
	if strings.Contains(string(signingJSON), "old-public-key") || strings.Contains(string(signingJSON), `"enabled":true`) {
		t.Errorf("seeded signing.json leaked into the output: %s", signingJSON)
	}
}

func TestBuildSiteSeedDirSignedWarnsMissingSignatures(t *testing.T) {
	tempDir := t.TempDir()
	seedDir := filepath.Join(tempDir, "seed")
	writeSeedIndex(t, seedDir, singleImageSeed())

	key, err := generateMockGPGKey()
	if err != nil {
		t.Fatalf("failed to generate mock GPG key: %v", err)
	}
	logPath := filepath.Join(tempDir, "build.log")
	if err := logger.InitFileLogging(logPath); err != nil {
		t.Fatalf("failed to start file logging: %v", err)
	}
	defer logger.CloseLogFile(false)

	err = BuildSite(SiteOptions{
		SeedDir:    seedDir,
		RecordsDir: filepath.Join(tempDir, "records"),
		SiteDir:    filepath.Join(tempDir, "site"),
		GPGKeys:    []string{key},
	})
	if err != nil {
		t.Fatalf("BuildSite failed for a signed seed without signatures: %v", err)
	}
	logger.CloseLogFile(false)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read build log: %v", err)
	}
	if !strings.Contains(string(data), "missing signature files for 1 ref(s)") {
		t.Errorf("build log does not warn about the missing signature: %s", data)
	}
	if !strings.Contains(string(data), "app/org.example.app/x86_64/stable") {
		t.Errorf("build log does not name the unverifiable ref: %s", data)
	}
}

func TestBuildSiteSeedDirMissingIndex(t *testing.T) {
	tempDir := t.TempDir()
	seedDir := filepath.Join(tempDir, "seed")
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		t.Fatalf("failed to create seed directory: %v", err)
	}

	siteDir := filepath.Join(tempDir, "site")
	err := BuildSite(SiteOptions{
		SeedDir:       seedDir,
		RecordsDir:    filepath.Join(tempDir, "records"),
		SiteDir:       siteDir,
		AllowUnsigned: true,
	})
	if err == nil || !strings.Contains(err.Error(), "failed to read seed index") {
		t.Fatalf("BuildSite error = %v, want a missing seed index error", err)
	}
	if _, statErr := os.Stat(siteDir); !os.IsNotExist(statErr) {
		t.Errorf("failed seed left output at %s: %v", siteDir, statErr)
	}
}

func TestBuildSiteSeedDirInPlace(t *testing.T) {
	tempDir := t.TempDir()
	siteDir := filepath.Join(tempDir, "site")
	sigPath := filepath.Join(siteDir, "sigs", "example", "other@sha256="+strings.Repeat("b", 64), "signature-1")
	if err := os.MkdirAll(filepath.Dir(sigPath), 0755); err != nil {
		t.Fatalf("failed to create signature directory: %v", err)
	}
	if err := os.WriteFile(sigPath, []byte("existing-signature"), 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}
	seedPath := filepath.Join(siteDir, "index", "static")
	if err := os.MkdirAll(filepath.Dir(seedPath), 0755); err != nil {
		t.Fatalf("failed to create index directory: %v", err)
	}
	seedData, err := json.Marshal(FlatpakIndex{
		Registry: "ghcr.io",
		Results: []IndexResultPackage{{
			Name: "example/other",
			Images: []IndexImage{{
				Digest:       "sha256:" + strings.Repeat("b", 64),
				Architecture: "amd64",
				Labels: map[string]string{
					"org.flatpak.ref":      "app/org.example.other/x86_64/nightly",
					"org.flatpak.metadata": "[Application]\nname=org.example.other",
				},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("failed to marshal seed index: %v", err)
	}
	if err := os.WriteFile(seedPath, seedData, 0644); err != nil {
		t.Fatalf("failed to write seed index: %v", err)
	}

	err = BuildSite(SiteOptions{
		SeedDir:       siteDir,
		SiteDir:       siteDir,
		RecordsDir:    filepath.Join(tempDir, "records"),
		AllowUnsigned: true,
	})
	if err != nil {
		t.Fatalf("BuildSite failed for an in-place seed: %v", err)
	}

	got, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatalf("failed to read existing signature: %v", err)
	}
	if string(got) != "existing-signature" {
		t.Errorf("in-place signature = %q, want it untouched", got)
	}
}

func writeSeedIndex(t *testing.T, seedDir string, index FlatpakIndex) {
	t.Helper()
	path := filepath.Join(seedDir, "index", "static")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create seed index directory: %v", err)
	}
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("failed to marshal seed index: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write seed index: %v", err)
	}
}

func singleImageSeed() FlatpakIndex {
	return FlatpakIndex{
		Registry: "ghcr.io",
		Results: []IndexResultPackage{{
			Name: "example/app",
			Images: []IndexImage{{
				Digest:       "sha256:" + strings.Repeat("a", 64),
				Architecture: "amd64",
				Labels: map[string]string{
					"org.flatpak.ref":      "app/org.example.app/x86_64/stable",
					"org.flatpak.metadata": "[Application]\nname=org.example.app",
				},
			}},
		}},
	}
}

func TestBuildSiteSeedDirUnsignedWithoutSignatures(t *testing.T) {
	tempDir := t.TempDir()
	seedDir := filepath.Join(tempDir, "seed")
	siteDir := filepath.Join(tempDir, "site")
	writeSeedIndex(t, seedDir, singleImageSeed())

	err := BuildSite(SiteOptions{
		SeedDir:       seedDir,
		RecordsDir:    filepath.Join(tempDir, "records"),
		SiteDir:       siteDir,
		AllowUnsigned: true,
	})
	if err != nil {
		t.Fatalf("BuildSite failed for an unsigned seed without signatures: %v", err)
	}
	if _, err := os.Stat(filepath.Join(siteDir, "index", "static")); err != nil {
		t.Errorf("index was not written: %v", err)
	}
}

func TestBuildSiteSeedDirSignedWithoutSignaturesProceeds(t *testing.T) {
	tempDir := t.TempDir()
	seedDir := filepath.Join(tempDir, "seed")
	siteDir := filepath.Join(tempDir, "site")
	writeSeedIndex(t, seedDir, singleImageSeed())

	key, err := generateMockGPGKey()
	if err != nil {
		t.Fatalf("failed to generate mock GPG key: %v", err)
	}
	err = BuildSite(SiteOptions{
		SeedDir:    seedDir,
		RecordsDir: filepath.Join(tempDir, "records"),
		SiteDir:    siteDir,
		GPGKeys:    []string{key},
	})
	if err != nil {
		t.Fatalf("BuildSite failed for a signed seed without signatures: %v", err)
	}
	if _, err := os.Stat(filepath.Join(siteDir, "index", "static")); err != nil {
		t.Errorf("index was not written: %v", err)
	}
}

func TestBuildSiteSeedDirDryRunWritesNoSignatures(t *testing.T) {
	tempDir := t.TempDir()
	seedDir := filepath.Join(tempDir, "seed")
	siteDir := filepath.Join(tempDir, "site")
	writeSeedIndex(t, seedDir, singleImageSeed())

	sigPath := filepath.Join(seedDir, "sigs", "example", "app@sha256="+strings.Repeat("a", 64), "signature-1")
	if err := os.MkdirAll(filepath.Dir(sigPath), 0755); err != nil {
		t.Fatalf("failed to create seed signature directory: %v", err)
	}
	if err := os.WriteFile(sigPath, []byte("seed-signature"), 0644); err != nil {
		t.Fatalf("failed to write seed signature: %v", err)
	}

	err := BuildSite(SiteOptions{
		SeedDir:    seedDir,
		RecordsDir: filepath.Join(tempDir, "records"),
		SiteDir:    siteDir,
		DryRun:     true,
		NoSign:     true,
	})
	if err != nil {
		t.Fatalf("BuildSite dry run failed: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(siteDir, "sigs")); !os.IsNotExist(statErr) {
		t.Errorf("dry run wrote signatures: %v", statErr)
	}
}

func TestBuildSiteSeedDirRejectsNonRegularFile(t *testing.T) {
	tempDir := t.TempDir()
	seedDir := filepath.Join(tempDir, "seed")
	writeSeedIndex(t, seedDir, singleImageSeed())

	sigsDir := filepath.Join(seedDir, "sigs")
	if err := os.MkdirAll(sigsDir, 0755); err != nil {
		t.Fatalf("failed to create seed signature directory: %v", err)
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(sigsDir, "leak")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	err := BuildSite(SiteOptions{
		SeedDir:       seedDir,
		RecordsDir:    filepath.Join(tempDir, "records"),
		SiteDir:       filepath.Join(tempDir, "site"),
		AllowUnsigned: true,
	})
	if err == nil || !strings.Contains(err.Error(), "non-regular file") {
		t.Fatalf("BuildSite error = %v, want a non-regular file error", err)
	}
}

func TestBuildSiteSeedDirSignaturesNotDirectory(t *testing.T) {
	tempDir := t.TempDir()
	seedDir := filepath.Join(tempDir, "seed")
	writeSeedIndex(t, seedDir, singleImageSeed())
	if err := os.WriteFile(filepath.Join(seedDir, "sigs"), []byte("not a directory"), 0644); err != nil {
		t.Fatalf("failed to write sigs file: %v", err)
	}

	err := BuildSite(SiteOptions{
		SeedDir:       seedDir,
		RecordsDir:    filepath.Join(tempDir, "records"),
		SiteDir:       filepath.Join(tempDir, "site"),
		AllowUnsigned: true,
	})
	if err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("BuildSite error = %v, want a not-a-directory error", err)
	}
}

func TestBuildSiteSeedDirRejectsEscapingLookaside(t *testing.T) {
	tempDir := t.TempDir()
	seedDir := filepath.Join(tempDir, "seed")
	writeSeedIndex(t, seedDir, singleImageSeed())

	outside := filepath.Join(tempDir, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "sub"), 0755); err != nil {
		t.Fatalf("failed to create outside directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "sub", "secret"), []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(seedDir, "sigs")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	err := BuildSite(SiteOptions{
		SeedDir:       seedDir,
		RecordsDir:    filepath.Join(tempDir, "records"),
		SiteDir:       filepath.Join(tempDir, "site"),
		AllowUnsigned: true,
	})
	if err == nil || !strings.Contains(err.Error(), "escape the seed site directory") {
		t.Fatalf("BuildSite error = %v, want an escaping lookaside error", err)
	}
}

func TestBuildSiteSeedDirDryRunValidatesLookaside(t *testing.T) {
	tempDir := t.TempDir()
	seedDir := filepath.Join(tempDir, "seed")
	writeSeedIndex(t, seedDir, singleImageSeed())
	if err := os.WriteFile(filepath.Join(seedDir, "sigs"), []byte("not a directory"), 0644); err != nil {
		t.Fatalf("failed to write sigs file: %v", err)
	}

	err := BuildSite(SiteOptions{
		SeedDir:    seedDir,
		RecordsDir: filepath.Join(tempDir, "records"),
		SiteDir:    filepath.Join(tempDir, "site"),
		DryRun:     true,
		NoSign:     true,
	})
	if err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("BuildSite error = %v, want the lookaside validated during a dry run", err)
	}
}

func TestCopyTreeSkipsNonRegular(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "regular"), []byte("data"), 0644); err != nil {
		t.Fatalf("failed to write regular file: %v", err)
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(srcDir, "link")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	if err := copyTree(srcDir, dstDir); err != nil {
		t.Fatalf("copyTree failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "regular")); err != nil {
		t.Errorf("regular file was not copied: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dstDir, "link")); !os.IsNotExist(err) {
		t.Errorf("non-regular file was copied: %v", err)
	}
}

func TestMissingSignatureRefs(t *testing.T) {
	siteDir := t.TempDir()
	present := filepath.Join(siteDir, "sigs", "example", "a@sha256="+strings.Repeat("a", 64), "signature-1")
	if err := os.MkdirAll(filepath.Dir(present), 0755); err != nil {
		t.Fatalf("failed to create signature directory: %v", err)
	}
	if err := os.WriteFile(present, []byte("sig"), 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	missingImage := func(pkg, digest, ref string) IndexResultPackage {
		return IndexResultPackage{Name: pkg, Images: []IndexImage{{
			Digest: digest,
			Labels: map[string]string{"org.flatpak.ref": ref, "org.flatpak.metadata": "meta"},
		}}}
	}
	index := FlatpakIndex{Results: []IndexResultPackage{
		missingImage("example/a", "sha256:"+strings.Repeat("a", 64), "app/a"),
		missingImage("example/b", "sha256:"+strings.Repeat("b", 64), "app/b"),
		missingImage("example/b", "sha256:"+strings.Repeat("b", 64), "app/b"), // duplicate
		{Name: "example/noref", Images: []IndexImage{{
			Digest: "sha256:" + strings.Repeat("c", 64),
			Labels: map[string]string{"org.flatpak.metadata": "meta"},
		}}},
		missingImage("../../../outside", "sha256:"+strings.Repeat("d", 64), "app/escape"),
		missingImage("example/sha512", "sha512:"+strings.Repeat("e", 64), "app/sha512"),
		missingImage("example/sha384", "sha384:"+strings.Repeat("f", 64), "app/sha384"),
		missingImage("example/badhex", "sha256:zzzz", "app/badhex"),
		{Name: "example/nometa", Images: []IndexImage{{
			Digest: "sha256:" + strings.Repeat("9", 64),
			Labels: map[string]string{"org.flatpak.ref": "app/nometa"},
		}}},
	}}

	got := missingSignatureRefs(index, siteDir, "sigs")
	want := []string{
		"app/b",
		"example/noref@sha256:" + strings.Repeat("c", 64),
		"app/escape",
		"app/sha512",
	}
	if !slices.Equal(got, want) {
		t.Errorf("missingSignatureRefs = %v, want %v", got, want)
	}
}

func TestLoadSeedIndexRejectsBothLocalSeeds(t *testing.T) {
	_, _, err := loadSeedIndex(SiteOptions{IndexFile: "index/static", SeedDir: "site"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("loadSeedIndex error = %v, want a mutually exclusive error", err)
	}
}

func TestCopyFileHardlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("payload"), 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	if err := os.Link(src, dst); err != nil {
		t.Skipf("hardlinks unsupported: %v", err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed for hardlinked files: %v", err)
	}
	for _, path := range []string{src, dst} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read %s: %v", path, err)
		}
		if string(got) != "payload" {
			t.Errorf("%s = %q, want payload", path, got)
		}
	}
}

func TestBuildSiteSeedFetchFailuresFallBackToRecords(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "server error", status: http.StatusInternalServerError, body: "boom"},
		{name: "unparseable body", status: http.StatusOK, body: "not-json"},
		{name: "missing results", status: http.StatusOK, body: `{"Registry":"ghcr.io"}`},
		{name: "null results", status: http.StatusOK, body: `{"Registry":"ghcr.io","Results":null}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			recordsDir := filepath.Join(tempDir, "records")
			siteDir := filepath.Join(tempDir, "site")

			rec := record.Record{
				AppID:    "org.example.app",
				Arch:     "x86_64",
				Branch:   "stable",
				Name:     "example/app",
				Registry: "ghcr.io",
				Digest:   "sha256:" + strings.Repeat("c", 64),
				Ref:      "app/org.example.app/x86_64/stable",
			}
			labels := map[string]string{
				"org.flatpak.ref":      "app/org.example.app/x86_64/stable",
				"org.flatpak.metadata": "[Application]\nname=org.example.app",
			}
			if _, err := record.WriteRecord(recordsDir, rec, labels); err != nil {
				t.Fatalf("failed to write record: %v", err)
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/index/static" {
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(tt.body))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			err := BuildSite(SiteOptions{
				PagesURL:      server.URL,
				RecordsDir:    recordsDir,
				SiteDir:       siteDir,
				AllowUnsigned: true,
			})
			if err != nil {
				t.Fatalf("BuildSite should fall back to records, got error: %v", err)
			}

			data, err := os.ReadFile(filepath.Join(siteDir, "index", "static"))
			if err != nil {
				t.Fatalf("failed to read index static: %v", err)
			}
			var index FlatpakIndex
			if err := json.Unmarshal(data, &index); err != nil {
				t.Fatalf("failed to parse index static: %v", err)
			}
			var refs []string
			for _, pkg := range index.Results {
				for _, img := range pkg.Images {
					refs = append(refs, img.Labels["org.flatpak.ref"])
				}
			}
			if want := []string{"app/org.example.app/x86_64/stable"}; !slices.Equal(refs, want) {
				t.Errorf("index refs = %v, want only the local record ref %v", refs, want)
			}
		})
	}
}

func TestDecodeSeedIndex(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{name: "valid", data: `{"Registry":"ghcr.io","Results":[]}`},
		{name: "empty results", data: `{"Results":[]}`},
		{name: "null document", data: `null`, wantErr: true},
		{name: "array document", data: `[]`, wantErr: true},
		{name: "missing results", data: `{"Registry":"ghcr.io"}`, wantErr: true},
		{name: "null results", data: `{"Registry":"ghcr.io","Results":null}`, wantErr: true},
		{name: "syntactically invalid", data: `{not-json`, wantErr: true},
		{name: "trailing data", data: `{"Results":[]} trailing`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, err := decodeSeedIndex([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeSeedIndex(%q) error = %v, wantErr %v", tt.data, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if index.Results == nil {
				t.Error("decoded Results is nil, want an empty slice")
			}
		})
	}
}

func TestLoadSeedIndexReportsUnavailableSeed(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "server error", status: http.StatusInternalServerError, body: "boom"},
		{name: "unparseable body", status: http.StatusOK, body: "not-json"},
		{name: "missing results", status: http.StatusOK, body: `{"Registry":"ghcr.io"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			index, warning, err := loadSeedIndex(SiteOptions{PagesURL: server.URL})
			if err != nil {
				t.Fatalf("loadSeedIndex returned an error for a Pages failure: %v", err)
			}
			if warning == "" {
				t.Fatal("loadSeedIndex swallowed the Pages seed failure instead of reporting it")
			}
			if !strings.Contains(warning, "Could not seed index") {
				t.Errorf("warning = %q, want it to name the failed seed", warning)
			}
			if len(index.Results) != 0 {
				t.Errorf("index has %d results, want an empty fallback index", len(index.Results))
			}
		})
	}
}

func TestBuildSiteReconcileActiveOCIRepository(t *testing.T) {
	// Setup a mock HTTP registry that returns 200 OK for everything (both apps exist in registry)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// Set up mock records:
	// App 1 in Active OCI Repository (example/app1)
	rec1 := record.Record{
		AppID:    "org.example.app1",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app1",
		Registry: server.URL,
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}
	labels1 := map[string]string{
		"org.flatpak.ref":      "app/org.example.app1/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app1",
	}
	if _, err := record.WriteRecord(recordsDir, rec1, labels1); err != nil {
		t.Fatalf("failed to write record 1: %v", err)
	}

	// App 2 in OLD OCI Repository (example/app1-old)
	rec2 := record.Record{
		AppID:    "org.example.app2",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app1-old",
		Registry: server.URL,
		Digest:   "sha256:2222222222222222222222222222222222222222222222222222222222222222",
	}
	labels2 := map[string]string{
		"org.flatpak.ref":      "app/org.example.app2/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app2",
	}
	if _, err := record.WriteRecord(recordsDir, rec2, labels2); err != nil {
		t.Fatalf("failed to write record 2: %v", err)
	}

	opts := SiteOptions{
		RecordsDir:          recordsDir,
		SiteDir:             siteDir,
		Reconcile:           true,
		ActiveAppIDs:        []string{"org.example.app1", "org.example.app2"},
		ActiveOCIRepository: "example/app1", // Only example/app1 is active!
		Insecure:            true,
		AllowUnsigned:       true,
	}

	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed with reconcile=true and ActiveOCIRepository: %v", err)
	}

	// Verify that index/static file was written and only contains app1 under example/app1, NOT under example/app1-old!
	indexPath := filepath.Join(siteDir, "index", "static")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index static: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "example/app1") {
		t.Error("expected index to contain example/app1")
	}
	if strings.Contains(content, "example/app1-old") {
		t.Error("expected index to NOT contain example/app1-old since it is not in ActiveOCIRepository")
	}
}

func TestBuildSiteReconcileEmptyActiveOCIRepository(t *testing.T) {
	// Setup a mock HTTP registry that returns 200 OK for everything
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// Set up mock records with different OCI repositories
	rec1 := record.Record{
		AppID:    "org.example.app1",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app1",
		Registry: server.URL,
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}
	labels1 := map[string]string{
		"org.flatpak.ref":      "app/org.example.app1/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app1",
	}
	if _, err := record.WriteRecord(recordsDir, rec1, labels1); err != nil {
		t.Fatalf("failed to write record 1: %v", err)
	}

	rec2 := record.Record{
		AppID:    "org.example.app2",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app2",
		Registry: server.URL,
		Digest:   "sha256:2222222222222222222222222222222222222222222222222222222222222222",
	}
	labels2 := map[string]string{
		"org.flatpak.ref":      "app/org.example.app2/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app2",
	}
	if _, err := record.WriteRecord(recordsDir, rec2, labels2); err != nil {
		t.Fatalf("failed to write record 2: %v", err)
	}

	opts := SiteOptions{
		RecordsDir:          recordsDir,
		SiteDir:             siteDir,
		Reconcile:           true,
		ActiveOCIRepository: "", // Empty: active OCI repo pruning is disabled!
		Insecure:            true,
		AllowUnsigned:       true,
	}

	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed with reconcile=true and empty ActiveOCIRepository: %v", err)
	}

	indexPath := filepath.Join(siteDir, "index", "static")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index static: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "example/app1") {
		t.Error("expected index to contain example/app1")
	}
	if !strings.Contains(content, "example/app2") {
		t.Error("expected index to contain example/app2 since ActiveOCIRepository was empty")
	}
}

func TestBuildSiteReconcileEmptyActiveAppIDs(t *testing.T) {
	// Setup a mock HTTP registry that returns 200 OK for everything
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// Set up mock records
	rec1 := record.Record{
		AppID:    "org.example.app1",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app1",
		Registry: server.URL,
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}
	labels1 := map[string]string{
		"org.flatpak.ref":      "app/org.example.app1/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app1",
	}
	if _, err := record.WriteRecord(recordsDir, rec1, labels1); err != nil {
		t.Fatalf("failed to write record 1: %v", err)
	}

	rec2 := record.Record{
		AppID:    "org.example.app2",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app1",
		Registry: server.URL,
		Digest:   "sha256:2222222222222222222222222222222222222222222222222222222222222222",
	}
	labels2 := map[string]string{
		"org.flatpak.ref":      "app/org.example.app2/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app2",
	}
	if _, err := record.WriteRecord(recordsDir, rec2, labels2); err != nil {
		t.Fatalf("failed to write record 2: %v", err)
	}

	opts := SiteOptions{
		RecordsDir:    recordsDir,
		SiteDir:       siteDir,
		Reconcile:     true,
		ActiveAppIDs:  nil, // Nil/Empty: active App IDs pruning is disabled!
		Insecure:      true,
		AllowUnsigned: true,
	}

	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed with reconcile=true and empty ActiveAppIDs: %v", err)
	}

	indexPath := filepath.Join(siteDir, "index", "static")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index static: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "org.example.app1") {
		t.Error("expected index to contain org.example.app1")
	}
	if !strings.Contains(content, "org.example.app2") {
		t.Error("expected index to contain org.example.app2 since ActiveAppIDs was empty")
	}
}

func TestBuildSiteReconcileAuthError(t *testing.T) {
	// Setup a mock HTTP registry that returns 401 Unauthorized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// Set up mock record
	rec := record.Record{
		AppID:    "org.example.app",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app",
		Registry: server.URL,
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}
	labels := map[string]string{
		"org.flatpak.ref":      "app/org.example.app/x86_64/stable",
		"org.flatpak.metadata": "[Application]\nname=org.example.app",
	}
	if _, err := record.WriteRecord(recordsDir, rec, labels); err != nil {
		t.Fatalf("failed to write record: %v", err)
	}

	opts := SiteOptions{
		RecordsDir:    recordsDir,
		SiteDir:       siteDir,
		Reconcile:     true,
		Insecure:      true,
		AllowUnsigned: true,
	}

	// BuildSite should succeed (it logs warning for 401/403 but does not prune the index entry)
	if err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite failed on auth error: %v", err)
	}

	// Verify that index/static contains org.example.app (it was NOT pruned!)
	indexPath := filepath.Join(siteDir, "index", "static")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index static: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "org.example.app") {
		t.Error("expected index to contain app since it should not be pruned on auth error")
	}
}

func TestWriteFlatpakRepoCustomSigDir(t *testing.T) {
	tempDir := t.TempDir()
	opts := SiteOptions{
		PagesURL:   "https://pages.example.com",
		RemoteName: "myremote",
		RepoTitle:  "My Repo",
		SigDir:     "custom-signatures",
	}

	err := writeFlatpakRepoFile(tempDir, "ghcr.io", "some-gpg-key-base64", opts)
	if err != nil {
		t.Fatalf("writeFlatpakRepoFile failed: %v", err)
	}

	repoFilePath := filepath.Join(tempDir, "myremote.flatpakrepo")
	repoBytes, err := os.ReadFile(repoFilePath)
	if err != nil {
		t.Fatalf("failed to read generated flatpakrepo file: %v", err)
	}
	repoContent := string(repoBytes)

	// Verify that SignatureLookaside uses custom-signatures
	expectedLookaside := "SignatureLookaside=https://pages.example.com/custom-signatures"
	if !strings.Contains(repoContent, expectedLookaside) {
		t.Errorf("expected flatpakrepo to contain %q, got:\n%s", expectedLookaside, repoContent)
	}
}

func TestWriteFlatpakRepoDefaultTitle(t *testing.T) {
	tests := []struct {
		name       string
		remoteName string
		repoTitle  string
		expected   string
	}{
		{
			name:       "defaults to remote name when title is empty",
			remoteName: "my-custom-remote",
			repoTitle:  "",
			expected:   "Title=my-custom-remote",
		},
		{
			name:       "defaults to aetherpak when both are empty",
			remoteName: "",
			repoTitle:  "",
			expected:   "Title=aetherpak",
		},
		{
			name:       "uses configured repo title override",
			remoteName: "my-custom-remote",
			repoTitle:  "Override Title",
			expected:   "Title=Override Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			opts := SiteOptions{
				RemoteName: tt.remoteName,
				RepoTitle:  tt.repoTitle,
			}

			err := writeFlatpakRepoFile(tempDir, "ghcr.io", "", opts)
			if err != nil {
				t.Fatalf("writeFlatpakRepoFile failed: %v", err)
			}

			remote := tt.remoteName
			if remote == "" {
				remote = "aetherpak"
			}
			repoFilePath := filepath.Join(tempDir, remote+".flatpakrepo")
			repoBytes, err := os.ReadFile(repoFilePath)
			if err != nil {
				t.Fatalf("failed to read generated flatpakrepo file: %v", err)
			}
			repoContent := string(repoBytes)

			if !strings.Contains(repoContent, tt.expected) {
				t.Errorf("expected flatpakrepo to contain %q, got:\n%s", tt.expected, repoContent)
			}
		})
	}
}

func TestBuildSiteDryRun(t *testing.T) {
	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// Set up mock record
	rec := record.Record{
		AppID:    "org.example.app",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app",
		Registry: "http://localhost:5000",
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}
	labels := map[string]string{
		"org.flatpak.ref": "app/org.example.app/x86_64/stable",
	}
	if _, err := record.WriteRecord(recordsDir, rec, labels); err != nil {
		t.Fatalf("failed to write record: %v", err)
	}

	opts := SiteOptions{
		RecordsDir:    recordsDir,
		SiteDir:       siteDir,
		Reconcile:     true,
		Insecure:      true,
		AllowUnsigned: true,
		DryRun:        true,
	}

	if err := BuildSite(opts); err != nil {
		t.Fatalf("expected dry-run BuildSite to succeed, got %v", err)
	}

	// Verify that siteDir is either empty or does not exist
	files, err := os.ReadDir(siteDir)
	if err == nil && len(files) > 0 {
		t.Errorf("expected no files to be written to site directory in dry-run mode, got %d files", len(files))
	}
}

func TestBuildSiteWithCompanions(t *testing.T) {
	tempDir := t.TempDir()
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// Set up mock records:
	// 1. Main app
	rec1 := record.Record{
		AppID:    "org.example.app",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app",
		Registry: "ghcr.io",
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}
	labels1 := map[string]string{
		"org.flatpak.ref":                   "app/org.example.app/x86_64/stable",
		"org.flatpak.metadata":              "[Application]\nname=org.example.app",
		"org.flatpak.timestamp":             "1717200000",
		"org.flatpak.installed-size":        "20971520",
		"org.flatpak.download-size":         "5242880",
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>Example App</name><summary>This is a main app</summary></component>`,
	}
	if _, err := record.WriteRecord(recordsDir, rec1, labels1); err != nil {
		t.Fatalf("failed to write record 1: %v", err)
	}

	// 2. Debug companion
	recDebug := record.Record{
		AppID:    "org.example.app.Debug",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app",
		Registry: "ghcr.io",
		Digest:   "sha256:111111111111111111111111111111111111111111111111111111111111111d",
	}
	labelsDebug := map[string]string{
		"org.flatpak.ref":            "runtime/org.example.app.Debug/x86_64/stable",
		"org.flatpak.metadata":       "[Runtime]\nname=org.example.app.Debug",
		"org.flatpak.timestamp":      "1717200000",
		"org.flatpak.installed-size": "314572800",
		"org.flatpak.download-size":  "78643200",
	}
	if _, err := record.WriteRecord(recordsDir, recDebug, labelsDebug); err != nil {
		t.Fatalf("failed to write debug record: %v", err)
	}

	// 3. Locale companion
	recLocale := record.Record{
		AppID:    "org.example.app.Locale",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "example/app",
		Registry: "ghcr.io",
		Digest:   "sha256:111111111111111111111111111111111111111111111111111111111111111l",
	}
	labelsLocale := map[string]string{
		"org.flatpak.ref":            "runtime/org.example.app.Locale/x86_64/stable",
		"org.flatpak.metadata":       "[Runtime]\nname=org.example.app.Locale",
		"org.flatpak.timestamp":      "1717200000",
		"org.flatpak.installed-size": "15728640",
		"org.flatpak.download-size":  "2097152",
	}
	if _, err := record.WriteRecord(recordsDir, recLocale, labelsLocale); err != nil {
		t.Fatalf("failed to write locale record: %v", err)
	}

	templateFile := filepath.Join(tempDir, "custom.html")
	templateContent := `{{range .Apps}}
App: {{.Name}} ({{.ID}})
{{range .Branches}}
- Branch: {{.Branch}}
{{range .Companions}}
  - Companion: {{.ID}} Type: {{.Type}} Arches: {{join .Arches "/"}} Install: {{.InstallCmd}} Ref: {{.RefFile}}
{{end}}
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

	// Verify that we only have ONE root app (org.example.app) listed in .Apps
	if strings.Count(output, "App:") != 1 {
		t.Errorf("expected exactly 1 root app to be outputted, found: %d", strings.Count(output, "App:"))
	}

	if !strings.Contains(output, "App: Example App (org.example.app)") {
		t.Errorf("expected output to contain parent app details, got:\n%s", output)
	}

	expectedDebugLine := "  - Companion: org.example.app.Debug Type: Debug Arches: x86_64 Install: flatpak install --user myremote org.example.app.Debug//stable Ref: refs/org.example.app.Debug-stable.flatpakref"
	if !strings.Contains(output, expectedDebugLine) {
		t.Errorf("expected output to contain debug companion details: %q", expectedDebugLine)
	}

	expectedLocaleLine := "  - Companion: org.example.app.Locale Type: Locale Arches: x86_64 Install: flatpak install --user myremote org.example.app.Locale//stable Ref: refs/org.example.app.Locale-stable.flatpakref"
	if !strings.Contains(output, expectedLocaleLine) {
		t.Errorf("expected output to contain locale companion details: %q", expectedLocaleLine)
	}
}

func TestExtensionIDsFromMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
		want     []string
	}{
		{
			name:     "empty metadata",
			metadata: "",
			want:     nil,
		},
		{
			name:     "application without extensions",
			metadata: "[Application]\nname=org.example.App",
			want:     nil,
		},
		{
			name: "declared extensions",
			metadata: "[Application]\nname=org.example.App\n" +
				"[Extension org.example.App.Plugin]\ndirectory=extensions/plugin\n" +
				"[Extension org.example.App.Plugin.Locale]\ndirectory=extensions/plugin/locale",
			want: []string{"org.example.App.Plugin", "org.example.App.Plugin.Locale"},
		},
		{
			name:     "tagged extension point drops the tag",
			metadata: "[Application]\nname=org.example.App\n[Extension org.example.App.Plugin@stable]\ndirectory=extensions/plugin",
			want:     []string{"org.example.App.Plugin"},
		},
		{
			name:     "crlf and whitespace",
			metadata: "[Application]\r\nname=org.example.App\r\n  [Extension org.example.App.Plugin]  \r\n",
			want:     []string{"org.example.App.Plugin"},
		},
		{
			name:     "other extension headers are ignored",
			metadata: "[Extension]\nname=org.example.Sdk.Extension.xrt\n[ExtensionOf]\nref=app/org.example.App/x86_64/stable\n[ExtensionFoo]",
			want:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extensionIDsFromMetadata(tt.metadata); !slices.Equal(got, tt.want) {
				t.Errorf("extensionIDsFromMetadata() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestActiveExtensionIDs(t *testing.T) {
	index := FlatpakIndex{Results: []IndexResultPackage{{
		Name: "example/app",
		Images: []IndexImage{
			{Labels: map[string]string{
				"org.flatpak.ref":      "app/org.example.app/x86_64/stable",
				"org.flatpak.metadata": "[Application]\n[Extension org.example.app.Plugin]\n[Extension org.example.app.Plugin@stable]",
			}},
			{Labels: map[string]string{
				"org.flatpak.ref":      "app/org.example.app/aarch64/stable",
				"org.flatpak.metadata": "[Application]\n[Extension org.example.app.Plugin]\n[Extension org.example.app.Arm]",
			}},
			{Labels: map[string]string{
				"org.flatpak.ref":      "app/org.example.other/x86_64/stable",
				"org.flatpak.metadata": "[Application]\n[Extension org.example.other.Plugin]",
			}},
			{Labels: map[string]string{
				"org.flatpak.ref":      "runtime/org.example.app.Plugin/x86_64/stable",
				"org.flatpak.metadata": "[Runtime]\n[Extension org.example.app.Plugin.Inner]",
			}},
			// A configured app can be published as a runtime bundle.
			{Labels: map[string]string{
				"org.flatpak.ref":      "runtime/org.example.app/x86_64/stable",
				"org.flatpak.metadata": "[Runtime]\n[Extension org.example.app.Runtime]",
			}},
		},
	}}}

	opts := SiteOptions{ActiveAppIDs: []string{"org.example.app"}}
	want := []string{"org.example.app.Arm", "org.example.app.Plugin", "org.example.app.Runtime"}
	if got := opts.activeExtensionIDs(index); !slices.Equal(got, want) {
		t.Errorf("activeExtensionIDs() = %v, want %v", got, want)
	}

	if got := (SiteOptions{}).activeExtensionIDs(index); got != nil {
		t.Errorf("activeExtensionIDs() with no active apps = %v, want nil", got)
	}
}
