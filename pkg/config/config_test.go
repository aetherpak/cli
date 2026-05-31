package config

import (
	"testing"
)

func TestAppValidate(t *testing.T) {
	tests := []struct {
		name    string
		app     App
		wantErr bool
	}{
		{
			name: "valid manifest app",
			app: App{
				ID:       "org.example.App",
				Branch:   "stable",
				Manifest: "apps/org.example.App.yaml",
				Runtime:  "gnome-40",
				Arches:   []string{"x86_64"},
			},
			wantErr: false,
		},
		{
			name: "invalid app id format",
			app: App{
				ID:       "invalid/app/id",
				Branch:   "stable",
				Manifest: "apps/app.yaml",
				Runtime:  "gnome-40",
			},
			wantErr: true,
		},
		{
			name: "invalid branch characters",
			app: App{
				ID:       "org.example.App",
				Branch:   "stable/release",
				Manifest: "apps/app.yaml",
				Runtime:  "gnome-40",
			},
			wantErr: true,
		},
		{
			name: "both manifest and bundles set",
			app: App{
				ID:       "org.example.App",
				Branch:   "stable",
				Manifest: "apps/app.yaml",
				Runtime:  "gnome-40",
				Bundles: map[string]Bundle{
					"x86_64": {URL: "https://example.com/b.flatpak", SHA256: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"},
				},
			},
			wantErr: true,
		},
		{
			name: "neither manifest nor bundles set",
			app: App{
				ID:     "org.example.App",
				Branch: "stable",
			},
			wantErr: true,
		},
		{
			name: "absolute manifest path",
			app: App{
				ID:       "org.example.App",
				Branch:   "stable",
				Manifest: "/etc/manifest.yaml",
				Runtime:  "gnome-40",
			},
			wantErr: true,
		},
		{
			name: "path traversal in manifest",
			app: App{
				ID:       "org.example.App",
				Branch:   "stable",
				Manifest: "../manifest.yaml",
				Runtime:  "gnome-40",
			},
			wantErr: true,
		},
		{
			name: "missing runtime for manifest source",
			app: App{
				ID:       "org.example.App",
				Branch:   "stable",
				Manifest: "apps/app.yaml",
			},
			wantErr: false,
		},
		{
			name: "valid bundle app",
			app: App{
				ID:     "org.example.App",
				Branch: "stable",
				Bundles: map[string]Bundle{
					"x86_64": {
						URL:    "https://example.com/app.flatpak",
						SHA256: "14152763261234567890abcdef1234567890abcdef1234567890abcdef123456",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "bundle URL missing scheme",
			app: App{
				ID:     "org.example.App",
				Branch: "stable",
				Bundles: map[string]Bundle{
					"x86_64": {
						URL:    "example.com/app.flatpak",
						SHA256: "14152763261234567890abcdef1234567890abcdef1234567890abcdef123456",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "bundle sha256 too short",
			app: App{
				ID:     "org.example.App",
				Branch: "stable",
				Bundles: map[string]Bundle{
					"x86_64": {
						URL:    "https://example.com/app.flatpak",
						SHA256: "1234567890abcdef",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.app.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("App.Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigNormalize(t *testing.T) {
	trueVal := true
	falseVal := false
	cfg := Config{
		Defaults: &DefaultsConfig{
			CCache:      &trueVal,
			CCacheDir:   "/global/ccache",
			StateDir:    "/global/state",
			RunLinter:   true,
			BuilderArgs: []string{"--foo", "--bar"},
		},
		Linter: &LinterConfig{
			Strict:      &falseVal,
			IgnoreRules: []string{"rule-1"},
		},
		Apps: []App{
			{
				ID:       "org.example.App1",
				Manifest: "apps/app1.yaml",
			},
			{
				ID:          "org.example.App2",
				Manifest:    "apps/app2.yaml",
				CCache:      &falseVal,
				CCacheDir:   "/local/ccache",
				BuilderArgs: []string{"--baz"},
				Linter: &LinterConfig{
					Strict:      &trueVal,
					IgnoreRules: []string{"rule-2"},
				},
			},
		},
	}

	cfg.Normalize()

	// App 1 should inherit global values
	app1 := cfg.Apps[0]
	if app1.CCache == nil || !*app1.CCache {
		t.Errorf("App1: expected CCache to be true (inherited), got %v", app1.CCache)
	}
	if app1.CCacheDir != "/global/ccache" {
		t.Errorf("App1: expected CCacheDir to be /global/ccache, got %q", app1.CCacheDir)
	}
	if app1.StateDir != "/global/state" {
		t.Errorf("App1: expected StateDir to be /global/state, got %q", app1.StateDir)
	}
	if !app1.RunLinter {
		t.Errorf("App1: expected RunLinter to be true")
	}
	if app1.Linter == nil || *app1.Linter.Strict != false || len(app1.Linter.IgnoreRules) != 1 || app1.Linter.IgnoreRules[0] != "rule-1" {
		t.Errorf("App1: expected Linter settings inherited, got %+v", app1.Linter)
	}
	if len(app1.BuilderArgs) != 2 || app1.BuilderArgs[0] != "--foo" || app1.BuilderArgs[1] != "--bar" {
		t.Errorf("App1: expected BuilderArgs inherited, got %v", app1.BuilderArgs)
	}

	// App 2 should preserve local values
	app2 := cfg.Apps[1]
	if app2.CCache == nil || *app2.CCache {
		t.Errorf("App2: expected CCache to be false (overridden), got %v", app2.CCache)
	}
	if app2.CCacheDir != "/local/ccache" {
		t.Errorf("App2: expected CCacheDir to be /local/ccache, got %q", app2.CCacheDir)
	}
	if app2.StateDir != "/global/state" {
		t.Errorf("App2: expected StateDir to be /global/state (inherited), got %q", app2.StateDir)
	}
	if app2.Linter == nil || *app2.Linter.Strict != true || len(app2.Linter.IgnoreRules) != 1 || app2.Linter.IgnoreRules[0] != "rule-2" {
		t.Errorf("App2: expected Linter settings overridden, got %+v", app2.Linter)
	}
	if len(app2.BuilderArgs) != 1 || app2.BuilderArgs[0] != "--baz" {
		t.Errorf("App2: expected BuilderArgs overridden/preserved, got %v", app2.BuilderArgs)
	}
}

func TestAppEqual(t *testing.T) {
	trueVal := true
	falseVal := false

	appA := App{
		ID:        "org.example.App",
		Branch:    "stable",
		Arches:    []string{"x86_64", "aarch64"},
		Manifest:  "apps/app.yaml",
		Runtime:   "gnome-40",
		RunLinter: true,
		Linter: &LinterConfig{
			Strict:      &trueVal,
			IgnoreRules: []string{"rule-1", "rule-2"},
		},
		CCache:      &trueVal,
		CCacheDir:   "/ccache",
		StateDir:    "/state",
		BuilderArgs: []string{"--arg1"},
		Bundles: map[string]Bundle{
			"x86_64": {URL: "https://example.com/b.flatpak", SHA256: "abcdef"},
		},
	}

	appB := appA
	if !appA.Equal(appB) {
		t.Error("identical App configs should be equal")
	}

	// Change string field
	appB.Branch = "beta"
	if appA.Equal(appB) {
		t.Error("differing Branch should not be equal")
	}

	// Reset and change slice
	appB = appA
	appB.Arches = []string{"x86_64"}
	if appA.Equal(appB) {
		t.Error("differing Arches should not be equal")
	}

	// Reset and change pointer bool
	appB = appA
	appB.CCache = &falseVal
	if appA.Equal(appB) {
		t.Error("differing CCache value should not be equal")
	}

	// Reset and change Linter
	appB = appA
	appB.Linter = &LinterConfig{
		Strict:      &trueVal,
		IgnoreRules: []string{"rule-1"},
	}
	if appA.Equal(appB) {
		t.Error("differing Linter should not be equal")
	}

	// Reset and change bundle
	appB = appA
	appB.Bundles = map[string]Bundle{
		"x86_64": {URL: "https://example.com/b.flatpak", SHA256: "different"},
	}
	if appA.Equal(appB) {
		t.Error("differing Bundles should not be equal")
	}
}
