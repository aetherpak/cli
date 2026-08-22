package appstream

import (
	"strings"
	"testing"
)

func TestSanitizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"v1.0.0", "1.0.0"},
		{"V2.3.4", "2.3.4"},
		{"v0.1.0-alpha.1", "0.1.0-alpha.1"},
		{"release-1.2.3", "1.2.3"},
		{"release_4.5.6", "4.5.6"},
		{"1.0.0", "1.0.0"},
		{"2026.08.22", "2026.08.22"},
		{"  v3.0.0  ", "3.0.0"},
		{"v-beta", "v-beta"}, // no digit after v, keep intact
	}

	for _, tt := range tests {
		actual := SanitizeVersion(tt.input)
		if actual != tt.expected {
			t.Errorf("SanitizeVersion(%q) = %q; want %q", tt.input, actual, tt.expected)
		}
	}
}

func TestHasRelease(t *testing.T) {
	xmlContent := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>org.example.App</id>
  <releases>
    <release version="1.2.0" date="2026-01-01"/>
    <release version="1.1.0" date="2025-06-01"/>
  </releases>
</component>`)

	has12, err := HasRelease(xmlContent, "1.2.0")
	if err != nil || !has12 {
		t.Errorf("expected HasRelease(1.2.0) = true, got %v, err: %v", has12, err)
	}

	has12V, err := HasRelease(xmlContent, "v1.2.0")
	if err != nil || !has12V {
		t.Errorf("expected HasRelease(v1.2.0) = true (sanitized), got %v, err: %v", has12V, err)
	}

	has13, err := HasRelease(xmlContent, "1.3.0")
	if err != nil || has13 {
		t.Errorf("expected HasRelease(1.3.0) = false, got %v, err: %v", has13, err)
	}
}

func TestSyncRelease_ExistingReleases(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!-- AppStream Metadata -->
<component type="desktop-application">
  <id>org.example.App</id>
  <name>Example App</name>
  <releases>
    <release version="1.0.0" date="2025-01-01">
      <description><p>Initial release.</p></description>
    </release>
  </releases>
</component>`)

	opts := ReleaseOptions{
		Version: "v1.1.0",
		Date:    "2026-08-22",
	}

	updated, modified, err := SyncRelease(input, opts)
	if err != nil {
		t.Fatalf("SyncRelease failed: %v", err)
	}
	if !modified {
		t.Fatalf("expected modified=true")
	}

	resStr := string(updated)
	if !strings.Contains(resStr, `<release version="1.1.0" date="2026-08-22"/>`) {
		t.Errorf("expected release tag in output, got:\n%s", resStr)
	}
	if !strings.Contains(resStr, `<!-- AppStream Metadata -->`) {
		t.Errorf("expected comments preserved, got:\n%s", resStr)
	}
	if !strings.Contains(resStr, `<release version="1.0.0" date="2025-01-01">`) {
		t.Errorf("expected existing release preserved, got:\n%s", resStr)
	}

	// Verify order: 1.1.0 appears before 1.0.0
	idx11 := strings.Index(resStr, `version="1.1.0"`)
	idx10 := strings.Index(resStr, `version="1.0.0"`)
	if idx11 >= idx10 {
		t.Errorf("expected version 1.1.0 to precede 1.0.0 in releases block")
	}
}

func TestSyncRelease_Idempotency(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>org.example.App</id>
  <releases>
    <release version="1.1.0" date="2026-08-22">
      <description><p>Manual detailed release notes.</p></description>
    </release>
  </releases>
</component>`)

	opts := ReleaseOptions{
		Version: "1.1.0",
		Date:    "2026-08-22",
	}

	updated, modified, err := SyncRelease(input, opts)
	if err != nil {
		t.Fatalf("SyncRelease failed: %v", err)
	}
	if modified {
		t.Errorf("expected modified=false for existing version")
	}
	if string(updated) != string(input) {
		t.Errorf("expected byte-for-byte identical output on idempotent call")
	}
}

func TestSyncRelease_NoReleasesBlock(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>org.example.App</id>
  <name>Example App</name>
  <summary>Simple app</summary>
</component>`)

	opts := ReleaseOptions{
		Version:     "v1.0.0",
		Date:        "2026-08-22",
		Description: "First release with feature X",
		URL:         "https://example.com/releases/1.0.0",
	}

	updated, modified, err := SyncRelease(input, opts)
	if err != nil {
		t.Fatalf("SyncRelease failed: %v", err)
	}
	if !modified {
		t.Fatalf("expected modified=true")
	}

	resStr := string(updated)
	if !strings.Contains(resStr, `<releases>`) || !strings.Contains(resStr, `</releases>`) {
		t.Errorf("expected <releases> container created, got:\n%s", resStr)
	}
	if !strings.Contains(resStr, `<release version="1.0.0" date="2026-08-22">`) {
		t.Errorf("expected release start tag, got:\n%s", resStr)
	}
	if !strings.Contains(resStr, `<p>First release with feature X</p>`) {
		t.Errorf("expected description paragraph, got:\n%s", resStr)
	}
	if !strings.Contains(resStr, `<url>https://example.com/releases/1.0.0</url>`) {
		t.Errorf("expected URL node, got:\n%s", resStr)
	}
}

func TestSyncRelease_SelfClosingReleases(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>org.example.App</id>
  <releases/>
</component>`)

	opts := ReleaseOptions{
		Version: "2.0.0",
		Date:    "2026-08-22",
	}

	updated, modified, err := SyncRelease(input, opts)
	if err != nil {
		t.Fatalf("SyncRelease failed: %v", err)
	}
	if !modified {
		t.Fatalf("expected modified=true")
	}

	resStr := string(updated)
	if !strings.Contains(resStr, `<releases>`) || !strings.Contains(resStr, `</releases>`) {
		t.Errorf("expected expanded <releases>...</releases>, got:\n%s", resStr)
	}
	if !strings.Contains(resStr, `<release version="2.0.0" date="2026-08-22"/>`) {
		t.Errorf("expected release tag, got:\n%s", resStr)
	}
}

func TestSyncRelease_TabIndentation(t *testing.T) {
	input := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<component type=\"desktop-application\">\n\t<id>org.example.App</id>\n\t<name>Example App</name>\n</component>")

	opts := ReleaseOptions{
		Version: "1.0.0",
		Date:    "2026-08-22",
	}

	updated, modified, err := SyncRelease(input, opts)
	if err != nil {
		t.Fatalf("SyncRelease failed: %v", err)
	}
	if !modified {
		t.Fatalf("expected modified=true")
	}

	resStr := string(updated)
	if !strings.Contains(resStr, "\t<releases>\n\t\t<release version=\"1.0.0\" date=\"2026-08-22\"/>\n\t</releases>") {
		t.Errorf("expected tab-indented releases block, got:\n%s", resStr)
	}
}

func TestSyncRelease_CustomClosingTag(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>org.example.App</id>
</component >`)

	opts := ReleaseOptions{
		Version: "1.0.0",
		Date:    "2026-08-22",
	}

	updated, modified, err := SyncRelease(input, opts)
	if err != nil {
		t.Fatalf("SyncRelease failed: %v", err)
	}
	if !modified {
		t.Fatalf("expected modified=true")
	}

	resStr := string(updated)
	if !strings.HasSuffix(strings.TrimSpace(resStr), "</component >") {
		t.Errorf("expected original closing tag preserved at end, got:\n%s", resStr)
	}
	if !strings.Contains(resStr, "<releases>\n    <release version=\"1.0.0\" date=\"2026-08-22\"/>\n  </releases>") {
		t.Errorf("expected releases block inserted cleanly, got:\n%s", resStr)
	}
}

func TestSyncRelease_ReleasesWithComment(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>org.example.App</id>
  <releases><!-- preserved comment --></releases>
</component>`)

	opts := ReleaseOptions{
		Version: "1.0.0",
		Date:    "2026-08-22",
	}

	updated, modified, err := SyncRelease(input, opts)
	if err != nil {
		t.Fatalf("SyncRelease failed: %v", err)
	}
	if !modified {
		t.Fatalf("expected modified=true")
	}

	resStr := string(updated)
	if !strings.Contains(resStr, "<!-- preserved comment -->") {
		t.Errorf("expected inner comment preserved, got:\n%s", resStr)
	}
	if !strings.Contains(resStr, `<release version="1.0.0" date="2026-08-22"/>`) {
		t.Errorf("expected release tag inserted, got:\n%s", resStr)
	}
}

func TestSyncRelease_ReleasesWithAttributeContainingAngleBracket(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>org.example.App</id>
  <releases data="a>b">
    <release version="0.9.0" date="2024-01-01"/>
  </releases>
</component>`)

	opts := ReleaseOptions{
		Version: "1.0.0",
		Date:    "2026-08-22",
	}

	updated, modified, err := SyncRelease(input, opts)
	if err != nil {
		t.Fatalf("SyncRelease failed: %v", err)
	}
	if !modified {
		t.Fatalf("expected modified=true")
	}

	resStr := string(updated)
	if !strings.Contains(resStr, `<releases data="a>b">`) {
		t.Errorf("expected original releases tag with attribute preserved, got:\n%s", resStr)
	}
	if !strings.Contains(resStr, `<release version="1.0.0" date="2026-08-22"/>`) {
		t.Errorf("expected new release inserted, got:\n%s", resStr)
	}
	if strings.Contains(resStr, `<releases data="a>`+"\n") {
		t.Errorf("expected release tag not to be split inside attribute value, got:\n%s", resStr)
	}
}
