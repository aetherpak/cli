package configedit

import (
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/config"
)

func TestAppendAppPreservesComments(t *testing.T) {
	existing := "# top comment\nregistry: ghcr.io/me   # inline\napps:\n  - id: org.existing.App   # first app\n    manifest: existing.yaml\n"
	app := config.App{ID: "org.new.App", Branch: "stable", Arches: []string{"x86_64"}, Manifest: "org.new.App.yaml"}

	out, err := AppendApp([]byte(existing), app)
	if err != nil {
		t.Fatalf("AppendApp: %v", err)
	}
	s := string(out)
	for _, want := range []string{"# top comment", "# inline", "# first app", "org.existing.App", "org.new.App", "org.new.App.yaml"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
	// run-linter should not be emitted for the new entry.
	if strings.Contains(s, "run-linter") {
		t.Errorf("unexpected run-linter in output:\n%s", s)
	}
}

func TestAppendAppNewFile(t *testing.T) {
	app := config.App{ID: "org.new.App", Branch: "stable", Arches: []string{"x86_64"}, Manifest: "m.yaml"}
	out, err := AppendApp(nil, app)
	if err != nil {
		t.Fatalf("AppendApp: %v", err)
	}
	if !strings.Contains(string(out), "apps:") || !strings.Contains(string(out), "org.new.App") {
		t.Errorf("new file missing apps/app:\n%s", out)
	}
}

func TestAppendAppNullAppsKey(t *testing.T) {
	// A config with a bare "apps:" (null value) must append successfully, not error.
	existing := "registry: ghcr.io/me\napps:\n"
	app := config.App{ID: "org.new.App", Branch: "stable", Arches: []string{"x86_64"}, Manifest: "m.yaml"}
	out, err := AppendApp([]byte(existing), app)
	if err != nil {
		t.Fatalf("AppendApp on null apps: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "org.new.App") || !strings.Contains(s, "registry: ghcr.io/me") {
		t.Errorf("unexpected output:\n%s", s)
	}
}

func TestAppendAppBundle(t *testing.T) {
	app := config.App{
		ID:     "org.new.App",
		Branch: "stable",
		Bundles: map[string]config.Bundle{
			"x86_64": {URL: "https://e/x.flatpak", SHA256: strings.Repeat("a", 64)},
		},
	}
	out, err := AppendApp(nil, app)
	if err != nil {
		t.Fatalf("AppendApp: %v", err)
	}
	if !strings.Contains(string(out), "bundles:") || !strings.Contains(string(out), "x86_64") {
		t.Errorf("bundle output wrong:\n%s", out)
	}
}

func TestAppendAppEmitsOptions(t *testing.T) {
	ccache := true
	app := config.App{
		ID:          "org.new.App",
		Branch:      "stable",
		Arches:      []string{"x86_64"},
		Manifest:    "m.yaml",
		RunLinter:   true,
		CCache:      &ccache,
		BuilderArgs: []string{"--install-deps-from=flathub"},
	}
	out, err := AppendApp(nil, app)
	if err != nil {
		t.Fatalf("AppendApp: %v", err)
	}
	s := string(out)
	for _, want := range []string{"run-linter: true", "ccache: true", "builder_args:", "--install-deps-from=flathub"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
}

func TestHasApp(t *testing.T) {
	existing := "apps:\n  - id: org.existing.App\n    manifest: m.yaml\n"
	got, err := HasApp([]byte(existing), "org.existing.App")
	if err != nil {
		t.Fatalf("HasApp: %v", err)
	}
	if !got {
		t.Error("HasApp = false, want true")
	}
	got, err = HasApp([]byte(existing), "org.absent.App")
	if err != nil {
		t.Fatalf("HasApp: %v", err)
	}
	if got {
		t.Error("HasApp = true for absent id")
	}
}

func TestSetValuePreservesCommentsAndOrdering(t *testing.T) {
	existing := `# global comment
registry: ghcr.io/me  # inline
remote_name: original
apps:
  - id: org.first.App   # first app
    manifest: first.yaml
  - id: org.second.App
    manifest: second.yaml
`
	out, err := SetValue([]byte(existing), "remote_name", []string{"updated-remote"})
	if err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	s := string(out)

	// Comments must survive.
	for _, want := range []string{"# global comment", "# inline", "# first app"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing comment %q:\n%s", want, s)
		}
	}

	// Value must be updated.
	if !strings.Contains(s, "remote_name: updated-remote") {
		t.Errorf("expected updated remote_name, got:\n%s", s)
	}

	// Original key ordering: registry before remote_name before apps.
	regIdx := strings.Index(s, "registry:")
	remoteIdx := strings.Index(s, "remote_name:")
	appsIdx := strings.Index(s, "apps:")
	if regIdx >= remoteIdx || remoteIdx >= appsIdx {
		t.Errorf("key ordering not preserved: registry@%d remote_name@%d apps@%d\n%s", regIdx, remoteIdx, appsIdx, s)
	}

	// Apps must maintain their order.
	firstIdx := strings.Index(s, "org.first.App")
	secondIdx := strings.Index(s, "org.second.App")
	if firstIdx >= secondIdx {
		t.Errorf("app ordering not preserved: first@%d second@%d\n%s", firstIdx, secondIdx, s)
	}
}

func TestSetValueNestedKeyCreatesParent(t *testing.T) {
	existing := "registry: ghcr.io\n"
	out, err := SetValue([]byte(existing), "branding.logo_url", []string{"https://logo.png"})
	if err != nil {
		t.Fatalf("SetValue nested: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "branding:") {
		t.Errorf("expected branding parent key:\n%s", s)
	}
	if !strings.Contains(s, "logo_url: https://logo.png") {
		t.Errorf("expected logo_url value:\n%s", s)
	}
	// Original key must survive.
	if !strings.Contains(s, "registry: ghcr.io") {
		t.Errorf("original registry lost:\n%s", s)
	}
}

func TestSetValueBool(t *testing.T) {
	existing := "registry: ghcr.io\n"
	out, err := SetValue([]byte(existing), "no_sign", []string{"true"})
	if err != nil {
		t.Fatalf("SetValue bool: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "no_sign: true") {
		t.Errorf("expected no_sign: true, got:\n%s", s)
	}
}

func TestSetValueBoolRejectsInvalid(t *testing.T) {
	existing := "registry: ghcr.io\n"
	_, err := SetValue([]byte(existing), "no_sign", []string{"banana"})
	if err == nil {
		t.Fatal("expected error for invalid boolean, got nil")
	}
	if !strings.Contains(err.Error(), "invalid boolean value") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSetValueListCommaSeparated(t *testing.T) {
	existing := "registry: ghcr.io\n"
	out, err := SetValue([]byte(existing), "linter.ignore_rules", []string{"rule1,rule2,rule3"})
	if err != nil {
		t.Fatalf("SetValue list: %v", err)
	}
	s := string(out)
	for _, want := range []string{"rule1", "rule2", "rule3"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing list item %q:\n%s", want, s)
		}
	}
}

func TestSetValueListMultipleArgs(t *testing.T) {
	existing := "registry: ghcr.io\n"
	out, err := SetValue([]byte(existing), "defaults.builder_args", []string{"--foo", "--bar"})
	if err != nil {
		t.Fatalf("SetValue list multi-arg: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "--foo") || !strings.Contains(s, "--bar") {
		t.Errorf("output missing list items:\n%s", s)
	}
}

func TestSetValueRejectsUnknownKey(t *testing.T) {
	existing := "registry: ghcr.io\n"
	_, err := SetValue([]byte(existing), "foobar", []string{"xyz"})
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown configuration key") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSetValueRejectsAppsKey(t *testing.T) {
	existing := "registry: ghcr.io\n"
	_, err := SetValue([]byte(existing), "apps", []string{"xyz"})
	if err == nil {
		t.Fatal("expected error for 'apps' key, got nil")
	}
}

func TestSetValueChannelMapping(t *testing.T) {
	existing := "registry: ghcr.io\n"
	out, err := SetValue([]byte(existing), "channel_mappings.main", []string{"stable"})
	if err != nil {
		t.Fatalf("SetValue channel_mappings: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "channel_mappings:") {
		t.Errorf("expected channel_mappings parent:\n%s", s)
	}
	if !strings.Contains(s, "main: stable") {
		t.Errorf("expected main: stable, got:\n%s", s)
	}
}

func TestSetValueUpdatesExistingNestedKey(t *testing.T) {
	existing := `registry: ghcr.io
branding:
  logo_url: https://old-logo.png
  accent_color: "#ff0000"
`
	out, err := SetValue([]byte(existing), "branding.logo_url", []string{"https://new-logo.png"})
	if err != nil {
		t.Fatalf("SetValue update nested: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "logo_url: https://new-logo.png") {
		t.Errorf("expected updated logo_url:\n%s", s)
	}
	// accent_color must survive.
	if !strings.Contains(s, "accent_color:") {
		t.Errorf("sibling key accent_color lost:\n%s", s)
	}
}

func TestSetValueNewFile(t *testing.T) {
	out, err := SetValue(nil, "registry", []string{"ghcr.io"})
	if err != nil {
		t.Fatalf("SetValue new file: %v", err)
	}
	if !strings.Contains(string(out), "registry: ghcr.io") {
		t.Errorf("unexpected output for new file:\n%s", out)
	}
}

func TestValidConfigKeysIsSorted(t *testing.T) {
	keys := ValidConfigKeys()
	if len(keys) == 0 {
		t.Fatal("no valid config keys returned")
	}
	for i := 1; i < len(keys); i++ {
		if keys[i] < keys[i-1] {
			t.Errorf("keys not sorted: %q comes after %q", keys[i], keys[i-1])
		}
	}
}
