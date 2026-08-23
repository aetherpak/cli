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
			NoInstallDeps: &trueVal,
			NoFlathub:     &falseVal,
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
				NoInstallDeps: &falseVal,
				NoFlathub:     &trueVal,
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
	if app1.NoInstallDeps == nil || !*app1.NoInstallDeps {
		t.Errorf("App1: expected NoInstallDeps to be true (inherited), got %v", app1.NoInstallDeps)
	}
	if app1.NoFlathub == nil || *app1.NoFlathub {
		t.Errorf("App1: expected NoFlathub to be false (inherited), got %v", app1.NoFlathub)
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
	if app2.NoInstallDeps == nil || *app2.NoInstallDeps {
		t.Errorf("App2: expected NoInstallDeps to be false (overridden), got %v", app2.NoInstallDeps)
	}
	if app2.NoFlathub == nil || !*app2.NoFlathub {
		t.Errorf("App2: expected NoFlathub to be true (overridden), got %v", app2.NoFlathub)
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
		NoInstallDeps: &trueVal,
		NoFlathub:     &falseVal,
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

	// Reset and change NoInstallDeps
	appB = appA
	appB.NoInstallDeps = &falseVal
	if appA.Equal(appB) {
		t.Error("differing NoInstallDeps value should not be equal")
	}

	// Reset and change NoFlathub
	appB = appA
	appB.NoFlathub = &trueVal
	if appA.Equal(appB) {
		t.Error("differing NoFlathub value should not be equal")
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

func TestZeroManifestSourcesParsing(t *testing.T) {
	yamlData := `
app_id: ai.lemonade_server.Lemonade
runtime: org.gnome.Platform//49
sources:
  binaries:
    - path: build/lemond
      dest: /app/bin/lemond
    - path: build/lemonade
      dest: /app/bin/lemonade
    - src/simple_binary
  desktop: data/lemonade-app.desktop
  metainfo: data/ai.lemonade_server.Lemonade.metainfo.xml
  icons: src/app/src-tauri/icons/
  files:
    - path: assets/
      dest: /app/share/lemonade/assets
`
	var cfg Config
	err := yaml.Unmarshal([]byte(yamlData), &cfg)
	if err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}

	cfg.Normalize()
	if len(cfg.Apps) != 1 {
		t.Fatalf("expected 1 normalized app, got %d", len(cfg.Apps))
	}

	app := cfg.Apps[0]
	if app.ID != "ai.lemonade_server.Lemonade" {
		t.Errorf("expected app ID ai.lemonade_server.Lemonade, got %q", app.ID)
	}
	if app.Runtime != "org.gnome.Platform" {
		t.Errorf("expected Runtime org.gnome.Platform, got %q", app.Runtime)
	}
	if app.RuntimeVersion != "49" {
		t.Errorf("expected RuntimeVersion 49, got %q", app.RuntimeVersion)
	}
	if app.SDK != "org.gnome.Sdk" {
		t.Errorf("expected SDK org.gnome.Sdk, got %q", app.SDK)
	}
	if app.SDKVersion != "49" {
		t.Errorf("expected SDKVersion 49, got %q", app.SDKVersion)
	}
	if app.Command != "lemond" {
		t.Errorf("expected Command lemond, got %q", app.Command)
	}
	if len(app.FinishArgs) == 0 {
		t.Errorf("expected default FinishArgs to be set")
	}

	if app.Sources == nil {
		t.Fatalf("expected Sources to not be nil")
	}
	if len(app.Sources.Binaries) != 3 {
		t.Fatalf("expected 3 binaries, got %d", len(app.Sources.Binaries))
	}
	if app.Sources.Binaries[0].Path != "build/lemond" || app.Sources.Binaries[0].Dest != "/app/bin/lemond" {
		t.Errorf("unexpected binary 0: %+v", app.Sources.Binaries[0])
	}
	if app.Sources.Binaries[1].Path != "build/lemonade" || app.Sources.Binaries[1].Dest != "/app/bin/lemonade" {
		t.Errorf("unexpected binary 1: %+v", app.Sources.Binaries[1])
	}
	if app.Sources.Binaries[2].Path != "src/simple_binary" || app.Sources.Binaries[2].Dest != "/app/bin/simple_binary" {
		t.Errorf("unexpected binary 2: %+v", app.Sources.Binaries[2])
	}

	if app.Sources.Desktop != "data/lemonade-app.desktop" {
		t.Errorf("expected desktop data/lemonade-app.desktop, got %q", app.Sources.Desktop)
	}
	if app.Sources.Metainfo != "data/ai.lemonade_server.Lemonade.metainfo.xml" {
		t.Errorf("expected metainfo data/ai.lemonade_server.Lemonade.metainfo.xml, got %q", app.Sources.Metainfo)
	}
	if app.Sources.Icons != "src/app/src-tauri/icons/" {
		t.Errorf("expected icons src/app/src-tauri/icons/, got %q", app.Sources.Icons)
	}
	if len(app.Sources.Files) != 1 || app.Sources.Files[0].Path != "assets/" || app.Sources.Files[0].Dest != "/app/share/lemonade/assets" {
		t.Errorf("unexpected files source: %+v", app.Sources.Files)
	}

	if err := app.Validate(); err != nil {
		t.Errorf("expected app.Validate() to pass, got: %v", err)
	}
}

func TestZeroManifestColonAndMapFormats(t *testing.T) {
	// 1. Test list of path/dest pairs and bare strings
	listYAML := `
app_id: co.nowledge.con
runtime: org.freedesktop.Platform//25.08
sources:
  binaries:
    - path: dist/con
      dest: /path/custom/con
    - path: dist/con-cli
      dest: /app/bin/con-cli
    - dist/con-daemon
  files:
    - path: assets/themes
      dest: /app/share/themes
`
	var cfg1 Config
	if err := yaml.Unmarshal([]byte(listYAML), &cfg1); err != nil {
		t.Fatalf("failed to unmarshal list yaml: %v", err)
	}
	cfg1.Normalize()
	app1 := cfg1.Apps[0]
	if len(app1.Sources.Binaries) != 3 {
		t.Fatalf("expected 3 binaries, got %d", len(app1.Sources.Binaries))
	}
	if app1.Sources.Binaries[0].Path != "dist/con" || app1.Sources.Binaries[0].Dest != "/path/custom/con" {
		t.Errorf("unexpected binary 0: %+v", app1.Sources.Binaries[0])
	}
	if app1.Sources.Binaries[1].Path != "dist/con-cli" || app1.Sources.Binaries[1].Dest != "/app/bin/con-cli" {
		t.Errorf("unexpected binary 1: %+v", app1.Sources.Binaries[1])
	}
	if app1.Sources.Binaries[2].Path != "dist/con-daemon" || app1.Sources.Binaries[2].Dest != "/app/bin/con-daemon" {
		t.Errorf("unexpected binary 2: %+v", app1.Sources.Binaries[2])
	}
	if len(app1.Sources.Files) != 1 || app1.Sources.Files[0].Path != "assets/themes" || app1.Sources.Files[0].Dest != "/app/share/themes" {
		t.Errorf("unexpected files 0: %+v", app1.Sources.Files[0])
	}

	// 2. Test dictionary mapping format
	mapYAML := `
app_id: co.nowledge.con
runtime: org.freedesktop.Platform//25.08
sources:
  binaries:
    dist/con: /app/bin/con
    dist/con-cli: /app/bin/con-cli
  files:
    assets/themes: /app/share/themes
  symlinks:
    /app/bin/con-cli: /app/bin/con-helper
`
	var cfg2 Config
	if err := yaml.Unmarshal([]byte(mapYAML), &cfg2); err != nil {
		t.Fatalf("failed to unmarshal map yaml: %v", err)
	}
	cfg2.Normalize()
	app2 := cfg2.Apps[0]
	if len(app2.Sources.Binaries) != 2 {
		t.Fatalf("expected 2 binaries from map, got %d", len(app2.Sources.Binaries))
	}
	if len(app2.Sources.Files) != 1 {
		t.Fatalf("expected 1 file from map, got %d", len(app2.Sources.Files))
	}
	if len(app2.Sources.Symlinks) != 1 {
		t.Fatalf("expected 1 symlink from map, got %d", len(app2.Sources.Symlinks))
	}
}

func TestZeroManifestValidation(t *testing.T) {
	tests := []struct {
		name    string
		app     App
		wantErr bool
	}{
		{
			name: "valid zero manifest app",
			app: App{
				ID:      "org.example.App",
				Runtime: "org.gnome.Platform//49",
				Sources: &SourcesConfig{
					Binaries: []BinarySource{
						{Path: "bin/app", Dest: "/app/bin/app"},
					},
					Desktop: "data/app.desktop",
				},
			},
			wantErr: false,
		},
		{
			name: "missing runtime",
			app: App{
				ID: "org.example.App",
				Sources: &SourcesConfig{
					Binaries: []BinarySource{
						{Path: "bin/app", Dest: "/app/bin/app"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty sources",
			app: App{
				ID:      "org.example.App",
				Runtime: "org.gnome.Platform//49",
				Sources: &SourcesConfig{},
			},
			wantErr: true,
		},
		{
			name: "path traversal in binary",
			app: App{
				ID:      "org.example.App",
				Runtime: "org.gnome.Platform//49",
				Sources: &SourcesConfig{
					Binaries: []BinarySource{
						{Path: "../secret/bin", Dest: "/app/bin/bin"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "absolute path in binary",
			app: App{
				ID:      "org.example.App",
				Runtime: "org.gnome.Platform//49",
				Sources: &SourcesConfig{
					Binaries: []BinarySource{
						{Path: "/usr/bin/app", Dest: "/app/bin/app"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid binary destination not in /app/",
			app: App{
				ID:      "org.example.App",
				Runtime: "org.gnome.Platform//49",
				Sources: &SourcesConfig{
					Binaries: []BinarySource{
						{Path: "bin/app", Dest: "/usr/bin/app"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "path traversal in desktop",
			app: App{
				ID:      "org.example.App",
				Runtime: "org.gnome.Platform//49",
				Sources: &SourcesConfig{
					Desktop: "../data/app.desktop",
				},
			},
			wantErr: true,
		},
		{
			name: "path traversal in icons",
			app: App{
				ID:      "org.example.App",
				Runtime: "org.gnome.Platform//49",
				Sources: &SourcesConfig{
					Icons: "../icons/",
				},
			},
			wantErr: true,
		},
		{
			name: "multiple sources defined (manifest and sources)",
			app: App{
				ID:       "org.example.App",
				Manifest: "app.yaml",
				Runtime:  "org.gnome.Platform//49",
				Sources: &SourcesConfig{
					Desktop: "data/app.desktop",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := tt.app
			app.Normalize()
			err := app.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseRuntimeRef(t *testing.T) {
	cases := []struct {
		input       string
		wantRuntime string
		wantVersion string
	}{
		{"org.gnome.Platform//49", "org.gnome.Platform", "49"},
		{"org.gnome.Platform/x86_64/49", "org.gnome.Platform", "49"},
		{"org.freedesktop.Platform//24.08", "org.freedesktop.Platform", "24.08"},
		{"org.kde.Platform:6.7", "org.kde.Platform", "6.7"},
		{"org.gnome.Platform", "org.gnome.Platform", ""},
	}

	for _, c := range cases {
		rt, ver := ParseRuntimeRef(c.input)
		if rt != c.wantRuntime || ver != c.wantVersion {
			t.Errorf("ParseRuntimeRef(%q) = (%q, %q), want (%q, %q)", c.input, rt, ver, c.wantRuntime, c.wantVersion)
		}
	}
}

func TestZeroManifestBareFileDestinationDefault(t *testing.T) {
	app := App{
		ID:      "org.example.MyApp",
		Runtime: "org.gnome.Platform//49",
		Sources: &SourcesConfig{
			Files: []FileSource{
				{Path: "data/config.json"},
				{Path: "data/themes/"},
			},
		},
	}

	app.Normalize()

	if err := app.Validate(); err != nil {
		t.Fatalf("expected Validate() to pass after Normalize(), got: %v", err)
	}

	if app.Sources.Files[0].Dest != "/app/share/org.example.MyApp/config.json" {
		t.Errorf("expected default dest for file, got: %q", app.Sources.Files[0].Dest)
	}
	if app.Sources.Files[1].Dest != "/app/share/org.example.MyApp/themes" {
		t.Errorf("expected default dest for dir, got: %q", app.Sources.Files[1].Dest)
	}
}

func TestZeroManifestCommandInferenceMatching(t *testing.T) {
	app := App{
		ID:      "co.nowledge.con",
		Runtime: "org.freedesktop.Platform//25.08",
		Sources: &SourcesConfig{
			Binaries: []BinarySource{
				{Path: "dist/con-cli", Dest: "/app/bin/con-cli"},
				{Path: "dist/con", Dest: "/app/bin/con"},
				{Path: "dist/con-daemon", Dest: "/app/bin/con-daemon"},
			},
		},
	}

	app.Normalize()

	// Should infer "con" because it matches the last segment of co.nowledge.con
	if app.Command != "con" {
		t.Errorf("expected inferred command 'con', got: %q", app.Command)
	}
}

func TestSourcesEqualOrderInvariance(t *testing.T) {
	s1 := &SourcesConfig{
		Binaries: []BinarySource{
			{Path: "a", Dest: "/app/bin/a"},
			{Path: "b", Dest: "/app/bin/b"},
		},
		Files: []FileSource{
			{Path: "f1", Dest: "/app/share/f1"},
			{Path: "f2", Dest: "/app/share/f2"},
		},
		Symlinks: []string{
			"/app/bin/a: /app/bin/a-sym",
			"/app/bin/b: /app/bin/b-sym",
		},
	}

	s2 := &SourcesConfig{
		Binaries: []BinarySource{
			{Path: "b", Dest: "/app/bin/b"},
			{Path: "a", Dest: "/app/bin/a"},
		},
		Files: []FileSource{
			{Path: "f2", Dest: "/app/share/f2"},
			{Path: "f1", Dest: "/app/share/f1"},
		},
		Symlinks: []string{
			"/app/bin/b: /app/bin/b-sym",
			"/app/bin/a: /app/bin/a-sym",
		},
	}

	if !sourcesEqual(s1, s2) {
		t.Errorf("sourcesEqual returned false for reordered identical sources")
	}

	app1 := App{ID: "app", Runtime: "rt", Sources: s1}
	app2 := App{ID: "app", Runtime: "rt", Sources: s2}
	if !app1.Equal(app2) {
		t.Errorf("App.Equal returned false for reordered identical sources")
	}
}

func TestAutoReleaseMetadata(t *testing.T) {
	yamlStr := `
defaults:
  auto_release_metadata: true
apps:
  - id: org.example.AppOne
    manifest: apps/one.json
  - id: org.example.AppTwo
    manifest: apps/two.json
    auto_release_metadata: false
  - id: org.example.AppThree
    manifest: apps/three.json
    auto-release-metadata: true
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}
	cfg.Normalize()

	if cfg.Defaults.AutoReleaseMetadata == nil || !*cfg.Defaults.AutoReleaseMetadata {
		t.Errorf("expected defaults.auto_release_metadata to be true")
	}

	// AppOne inherits true from defaults
	if !cfg.Apps[0].ResolveAutoReleaseMetadata(cfg.Defaults) {
		t.Errorf("expected AppOne to inherit auto_release_metadata=true")
	}

	// AppTwo explicitly false
	if cfg.Apps[1].ResolveAutoReleaseMetadata(cfg.Defaults) {
		t.Errorf("expected AppTwo to have auto_release_metadata=false")
	}

	// AppThree uses kebab-case
	if !cfg.Apps[2].ResolveAutoReleaseMetadata(cfg.Defaults) {
		t.Errorf("expected AppThree to have auto_release_metadata=true")
	}
}

func TestAppEqual_AutoReleaseMetadata(t *testing.T) {
	trueVal := true
	falseVal := false

	app1 := App{ID: "org.example.App", AutoReleaseMetadata: &trueVal}
	app2 := App{ID: "org.example.App", AutoReleaseMetadata: &trueVal}
	app3 := App{ID: "org.example.App", AutoReleaseMetadata: &falseVal}
	app4 := App{ID: "org.example.App", AutoReleaseMetadata: nil}

	if !app1.Equal(app2) {
		t.Errorf("expected app1 and app2 to be equal")
	}
	if app1.Equal(app3) {
		t.Errorf("expected app1 and app3 not to be equal")
	}
	if app1.Equal(app4) {
		t.Errorf("expected app1 and app4 not to be equal")
	}
	if app4.Equal(app1) {
		t.Errorf("expected app4 and app1 not to be equal")
	}
}
