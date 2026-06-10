package site

import (
	"net/http"
	"net/http/httptest"
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
		"org.flatpak.metadata": "[Application]\nname=org.example.app1",
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
		"org.flatpak.metadata": "[Application]\nname=org.example.app2",
	}
	if _, err := record.WriteRecord(recordsDir, rec2, labels2); err != nil {
		t.Fatalf("failed to write record 2: %v", err)
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
		t.Error("expected index to NOT contain app2 since it is not in ActiveAppIDs")
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
