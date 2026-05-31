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
