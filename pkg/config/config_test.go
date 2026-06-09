package config

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
		{
			name: "invalid flatpak remote URL scheme",
			app: App{
				ID:       "org.example.App",
				Branch:   "stable",
				Manifest: "apps/app.yaml",
				Remotes: map[string]RemoteConfig{
					"flathub": {URL: "ftp://dl.flathub.org/repo/flathub.flatpakrepo"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty flatpak remote name",
			app: App{
				ID:       "org.example.App",
				Branch:   "stable",
				Manifest: "apps/app.yaml",
				Remotes: map[string]RemoteConfig{
					"": {URL: "https://dl.flathub.org/repo/flathub.flatpakrepo"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty flatpak remote URL",
			app: App{
				ID:       "org.example.App",
				Branch:   "stable",
				Manifest: "apps/app.yaml",
				Remotes: map[string]RemoteConfig{
					"flathub": {URL: ""},
				},
			},
			wantErr: true,
		},
		{
			name: "empty flatpak dependency remote",
			app: App{
				ID:       "org.example.App",
				Branch:   "stable",
				Manifest: "apps/app.yaml",
				Flatpaks: []FlatpakDep{
					{Remote: "", Ref: "org.gnome.Sdk//45"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty flatpak dependency ref",
			app: App{
				ID:       "org.example.App",
				Branch:   "stable",
				Manifest: "apps/app.yaml",
				Flatpaks: []FlatpakDep{
					{Remote: "flathub", Ref: ""},
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
			Remotes: map[string]RemoteConfig{
				"flathub": {URL: "https://dl.flathub.org/repo/flathub.flatpakrepo"},
				"repoA":   {URL: "https://example.com/repoA.flatpakrepo"},
			},
			Flatpaks: []FlatpakDep{
				{Remote: "flathub", Ref: "org.gnome.Sdk//45"},
			},
		},
		Linter: &LinterConfig{
			Strict:      &falseVal,
			IgnoreRules: []string{"rule-1"},
			Exceptions:  []string{"rule-ex1"},
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
					Exceptions:  []string{"rule-ex2"},
				},
				Remotes: map[string]RemoteConfig{
					"repoA": {URL: "https://example.com/repoA-overridden.flatpakrepo"},
					"repoB": {URL: "https://example.com/repoB.flatpakrepo"},
				},
				Flatpaks: []FlatpakDep{
					{Remote: "repoA", Ref: "org.gnome.Sdk.ExtensionA//45"},
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
	if app1.Linter == nil || *app1.Linter.Strict != false || len(app1.Linter.IgnoreRules) != 1 || app1.Linter.IgnoreRules[0] != "rule-1" || len(app1.Linter.Exceptions) != 1 || app1.Linter.Exceptions[0] != "rule-ex1" {
		t.Errorf("App1: expected Linter settings inherited, got %+v", app1.Linter)
	}
	if len(app1.BuilderArgs) != 2 || app1.BuilderArgs[0] != "--foo" || app1.BuilderArgs[1] != "--bar" {
		t.Errorf("App1: expected BuilderArgs inherited, got %v", app1.BuilderArgs)
	}
	if len(app1.Remotes) != 2 || app1.Remotes["flathub"].URL != "https://dl.flathub.org/repo/flathub.flatpakrepo" || app1.Remotes["repoA"].URL != "https://example.com/repoA.flatpakrepo" {
		t.Errorf("App1: expected Remotes inherited, got %v", app1.Remotes)
	}
	if len(app1.Flatpaks) != 1 || app1.Flatpaks[0].Remote != "flathub" || app1.Flatpaks[0].Ref != "org.gnome.Sdk//45" {
		t.Errorf("App1: expected Flatpaks inherited, got %v", app1.Flatpaks)
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
	if app2.Linter == nil || *app2.Linter.Strict != true || len(app2.Linter.IgnoreRules) != 2 || app2.Linter.IgnoreRules[0] != "rule-2" || app2.Linter.IgnoreRules[1] != "rule-1" || len(app2.Linter.Exceptions) != 2 || app2.Linter.Exceptions[0] != "rule-ex2" || app2.Linter.Exceptions[1] != "rule-ex1" {
		t.Errorf("App2: expected Linter settings merged, got %+v", app2.Linter)
	}
	if len(app2.BuilderArgs) != 1 || app2.BuilderArgs[0] != "--baz" {
		t.Errorf("App2: expected BuilderArgs overridden/preserved, got %v", app2.BuilderArgs)
	}
	if len(app2.Remotes) != 3 || app2.Remotes["flathub"].URL != "https://dl.flathub.org/repo/flathub.flatpakrepo" || app2.Remotes["repoA"].URL != "https://example.com/repoA-overridden.flatpakrepo" || app2.Remotes["repoB"].URL != "https://example.com/repoB.flatpakrepo" {
		t.Errorf("App2: expected Remotes merged/overridden, got %v", app2.Remotes)
	}
	if len(app2.Flatpaks) != 2 {
		t.Fatalf("App2: expected 2 Flatpaks, got %d: %v", len(app2.Flatpaks), app2.Flatpaks)
	}
	if app2.Flatpaks[0].Remote != "flathub" || app2.Flatpaks[0].Ref != "org.gnome.Sdk//45" {
		t.Errorf("App2: expected first Flatpak to be default flathub:org.gnome.Sdk//45, got %+v", app2.Flatpaks[0])
	}
	if app2.Flatpaks[1].Remote != "repoA" || app2.Flatpaks[1].Ref != "org.gnome.Sdk.ExtensionA//45" {
		t.Errorf("App2: expected second Flatpak to be local repoA:org.gnome.Sdk.ExtensionA//45, got %+v", app2.Flatpaks[1])
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
			Exceptions:  []string{"rule-ex1"},
		},
		CCache:      &trueVal,
		CCacheDir:   "/ccache",
		StateDir:    "/state",
		BuilderArgs: []string{"--arg1"},
		Bundles: map[string]Bundle{
			"x86_64": {URL: "https://example.com/b.flatpak", SHA256: "abcdef"},
		},
		Remotes: map[string]RemoteConfig{
			"flathub": {URL: "https://dl.flathub.org/repo/flathub.flatpakrepo"},
		},
		Flatpaks: []FlatpakDep{
			{Remote: "flathub", Ref: "org.gnome.Sdk//45"},
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
		Exceptions:  []string{"rule-ex1"},
	}
	if appA.Equal(appB) {
		t.Error("differing Linter ignore rules should not be equal")
	}

	// Reset and change Linter exceptions
	appB = appA
	appB.Linter = &LinterConfig{
		Strict:      &trueVal,
		IgnoreRules: []string{"rule-1", "rule-2"},
		Exceptions:  []string{"rule-ex2"},
	}
	if appA.Equal(appB) {
		t.Error("differing Linter exceptions should not be equal")
	}

	// Reset and change bundle
	appB = appA
	appB.Bundles = map[string]Bundle{
		"x86_64": {URL: "https://example.com/b.flatpak", SHA256: "different"},
	}
	if appA.Equal(appB) {
		t.Error("differing Bundles should not be equal")
	}

	// Reset and change Remotes
	appB = appA
	appB.Remotes = map[string]RemoteConfig{
		"flathub": {URL: "https://example.com/other.flatpakrepo"},
	}
	if appA.Equal(appB) {
		t.Error("differing Remotes values should not be equal")
	}

	// Reset and change Flatpaks
	appB = appA
	appB.Flatpaks = []FlatpakDep{
		{Remote: "flathub", Ref: "org.gnome.Sdk//46"},
	}
	if appA.Equal(appB) {
		t.Error("differing Flatpaks refs should not be equal")
	}
}

func TestRemoteConfigParsing(t *testing.T) {
	// 1. Test YAML Parsing
	yamlStr := `
remotes:
  flat_str: https://dl.flathub.org/repo/flathub.flatpakrepo
  exploded:
    url: https://example.com/repo.flatpakrepo
    gpg_verify: false
    gpg_key: "/path/to/key"
    sig_verify_url: "https://example.com/sig"
`
	var cfg struct {
		Remotes map[string]RemoteConfig `yaml:"remotes"`
	}
	err := yaml.Unmarshal([]byte(yamlStr), &cfg)
	if err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if len(cfg.Remotes) != 2 {
		t.Fatalf("expected 2 remotes, got %d", len(cfg.Remotes))
	}

	flatStr, ok := cfg.Remotes["flat_str"]
	if !ok {
		t.Errorf("missing flat_str remote")
	} else {
		if flatStr.URL != "https://dl.flathub.org/repo/flathub.flatpakrepo" {
			t.Errorf("expected URL https://dl.flathub.org/repo/flathub.flatpakrepo, got %q", flatStr.URL)
		}
		if flatStr.GPGVerify != nil {
			t.Errorf("expected GPGVerify to be nil, got %v", flatStr.GPGVerify)
		}
	}

	exploded, ok := cfg.Remotes["exploded"]
	if !ok {
		t.Errorf("missing exploded remote")
	} else {
		if exploded.URL != "https://example.com/repo.flatpakrepo" {
			t.Errorf("expected URL https://example.com/repo.flatpakrepo, got %q", exploded.URL)
		}
		if exploded.GPGVerify == nil || *exploded.GPGVerify != false {
			t.Errorf("expected GPGVerify to be false, got %v", exploded.GPGVerify)
		}
		if exploded.GPGKey != "/path/to/key" {
			t.Errorf("expected GPGKey /path/to/key, got %q", exploded.GPGKey)
		}
		if exploded.SigVerifyURL != "https://example.com/sig" {
			t.Errorf("expected SigVerifyURL https://example.com/sig, got %q", exploded.SigVerifyURL)
		}
	}

	// 2. Test JSON Parsing
	jsonStr := `{
		"remotes": {
			"flat_str": "https://dl.flathub.org/repo/flathub.flatpakrepo",
			"exploded": {
				"url": "https://example.com/repo.flatpakrepo",
				"gpg_verify": true,
				"gpg_key": "some-key",
				"sig_verify_url": "some-sig-url"
			}
		}
	}`
	var cfgJSON struct {
		Remotes map[string]RemoteConfig `json:"remotes"`
	}
	err = json.Unmarshal([]byte(jsonStr), &cfgJSON)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	flatStrJSON, ok := cfgJSON.Remotes["flat_str"]
	if !ok {
		t.Errorf("missing flat_str remote in JSON")
	} else if flatStrJSON.URL != "https://dl.flathub.org/repo/flathub.flatpakrepo" {
		t.Errorf("expected URL from JSON string, got %q", flatStrJSON.URL)
	}

	explodedJSON, ok := cfgJSON.Remotes["exploded"]
	if !ok {
		t.Errorf("missing exploded remote in JSON")
	} else {
		if explodedJSON.URL != "https://example.com/repo.flatpakrepo" {
			t.Errorf("expected URL from JSON object, got %q", explodedJSON.URL)
		}
		if explodedJSON.GPGVerify == nil || *explodedJSON.GPGVerify != true {
			t.Errorf("expected GPGVerify to be true in JSON, got %v", explodedJSON.GPGVerify)
		}
		if explodedJSON.GPGKey != "some-key" {
			t.Errorf("expected GPGKey some-key, got %q", explodedJSON.GPGKey)
		}
		if explodedJSON.SigVerifyURL != "some-sig-url" {
			t.Errorf("expected SigVerifyURL some-sig-url, got %q", explodedJSON.SigVerifyURL)
		}
	}

	// 3. Test Equal / String
	falseVal := false
	r1 := RemoteConfig{URL: "https://url", GPGVerify: &falseVal}
	r2 := RemoteConfig{URL: "https://url", GPGVerify: &falseVal}
	if !r1.Equal(r2) {
		t.Errorf("expected r1 to equal r2")
	}

	r3 := RemoteConfig{URL: "https://url", GPGVerify: &falseVal, GPGKey: "key"}
	if r1.Equal(r3) {
		t.Errorf("expected r1 to not equal r3")
	}

	str := r3.String()
	if !strings.Contains(str, "gpg_key=key") || !strings.Contains(str, "gpg_verify=false") {
		t.Errorf("expected string representation to contain key/verify, got %q", str)
	}
}
