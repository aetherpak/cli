package adder

import (
	"testing"

	"github.com/aetherpak/aetherpak/pkg/config"
)

func TestAppendArgDedup(t *testing.T) {
	got := appendArg([]string{"--a"}, "--a")
	if len(got) != 1 {
		t.Errorf("duplicate arg added: %v", got)
	}
	got = appendArg([]string{"--a"}, "--b")
	if len(got) != 2 || got[1] != "--b" {
		t.Errorf("new arg not appended: %v", got)
	}
}

func TestBoolOptionsDefaults(t *testing.T) {
	want := map[string]bool{
		"run-linter":                false,
		"install-deps-from-flathub": true,
		"ccache":                    false,
	}
	for _, o := range BoolOptions {
		if !o.AppliesTo(SourceManifest) {
			t.Errorf("%s should apply to manifest source", o.Key)
		}
		if o.AppliesTo(SourceBundle) {
			t.Errorf("%s must not apply to bundle source", o.Key)
		}
		if exp, ok := want[o.Key]; !ok {
			t.Errorf("unexpected option %q", o.Key)
		} else if o.Default != exp {
			t.Errorf("%s default = %v, want %v", o.Key, o.Default, exp)
		}
	}
}

func TestApplyOptionsManifest(t *testing.T) {
	app := config.App{ID: "org.x.App"}
	applyOptions(&app, SourceManifest, map[string]bool{
		"run-linter":                true,
		"ccache":                    true,
		"install-deps-from-flathub": true,
	}, nil)

	if !app.RunLinter {
		t.Error("run-linter not applied")
	}
	if app.CCache == nil || !*app.CCache {
		t.Error("ccache not applied")
	}
	if len(app.BuilderArgs) != 1 || app.BuilderArgs[0] != "--install-deps-from=flathub" {
		t.Errorf("flathub builder arg wrong: %v", app.BuilderArgs)
	}
}

func TestApplyOptionsFreeFormDedupesFlathub(t *testing.T) {
	app := config.App{ID: "org.x.App"}
	applyOptions(&app, SourceManifest,
		map[string]bool{"install-deps-from-flathub": true},
		[]string{"--install-deps-from=flathub", "--verbose"})
	if len(app.BuilderArgs) != 2 {
		t.Errorf("expected deduped 2 args, got %v", app.BuilderArgs)
	}
}

func TestApplyOptionsBundleSkipsBuilderOptions(t *testing.T) {
	app := config.App{ID: "org.x.App"}
	applyOptions(&app, SourceBundle, map[string]bool{
		"run-linter":                true,
		"install-deps-from-flathub": true,
	}, []string{"--verbose"})
	if app.RunLinter {
		t.Error("run-linter must not apply to bundle source")
	}
	if len(app.BuilderArgs) != 0 {
		t.Errorf("bundle source must not receive builder args: %v", app.BuilderArgs)
	}
}
