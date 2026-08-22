package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/config"
	"gopkg.in/yaml.v3"
)

func TestGenerateManifestYAML(t *testing.T) {
	tempDir := t.TempDir()

	// Create test icon directory structure
	iconsDir := filepath.Join(tempDir, "src", "app", "icons")
	if err := os.MkdirAll(iconsDir, 0755); err != nil {
		t.Fatalf("failed to create icons dir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(iconsDir, "128x128.png"), []byte("png128"), 0644)
	_ = os.WriteFile(filepath.Join(iconsDir, "scalable.svg"), []byte("<svg></svg>"), 0644)

	opts := GenerateOptions{
		AppID:          "ai.lemonade_server.Lemonade",
		Runtime:        "org.gnome.Platform",
		RuntimeVersion: "49",
		SDK:            "org.gnome.Sdk",
		SDKVersion:     "49",
		Binaries: []config.BinarySource{
			{Path: "build/lemond", Dest: "/app/bin/lemond"},
			{Path: "build/lemonade", Dest: "/app/bin/lemonade"},
		},
		Desktop:  "data/lemonade-app.desktop",
		Metainfo: "data/ai.lemonade_server.Lemonade.metainfo.xml",
		Icons:    iconsDir,
		Files: []config.FileSource{
			{Path: "assets/data", Dest: "/app/share/lemonade/data"},
		},
		Symlinks: []string{
			"/app/bin/lemonade: /app/bin/lemond",
		},
	}

	stateDir := filepath.Join(tempDir, ".state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	yamlBytes, err := GenerateManifestYAML(opts, stateDir)
	if err != nil {
		t.Fatalf("GenerateManifestYAML failed: %v", err)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		t.Fatalf("failed to parse generated manifest yaml: %v", err)
	}

	if raw["id"] != "ai.lemonade_server.Lemonade" {
		t.Errorf("expected id ai.lemonade_server.Lemonade, got %v", raw["id"])
	}
	if raw["runtime"] != "org.gnome.Platform" {
		t.Errorf("expected runtime org.gnome.Platform, got %v", raw["runtime"])
	}
	if raw["runtime-version"] != "49" {
		t.Errorf("expected runtime-version 49, got %v", raw["runtime-version"])
	}
	if raw["sdk"] != "org.gnome.Sdk" {
		t.Errorf("expected sdk org.gnome.Sdk, got %v", raw["sdk"])
	}
	if raw["sdk-version"] != "49" {
		t.Errorf("expected sdk-version 49, got %v", raw["sdk-version"])
	}
	if raw["command"] != "lemond" {
		t.Errorf("expected command lemond, got %v", raw["command"])
	}

	modules, ok := raw["modules"].([]interface{})
	if !ok || len(modules) == 0 {
		t.Fatalf("expected modules list")
	}

	mod0, ok := modules[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected module map")
	}

	if mod0["buildsystem"] != "simple" {
		t.Errorf("expected buildsystem simple, got %v", mod0["buildsystem"])
	}

	// Verify sources list dest-filename / dest uses base names
	sources, ok := mod0["sources"].([]interface{})
	if !ok || len(sources) == 0 {
		t.Fatalf("expected sources list")
	}

	for _, s := range sources {
		sm, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if df, ok := sm["dest-filename"].(string); ok {
			if strings.Contains(df, "/") || strings.Contains(df, "\\") {
				t.Errorf("dest-filename must be a plain base filename, got: %q", df)
			}
		}
		if d, ok := sm["dest"].(string); ok {
			if strings.Contains(d, "/") || strings.Contains(d, "\\") {
				t.Errorf("dest directory must be a plain base name, got: %q", d)
			}
		}
	}

	bCmds, ok := mod0["build-commands"].([]interface{})
	if !ok || len(bCmds) == 0 {
		t.Fatalf("expected build-commands list")
	}

	cmdStr := strings.Join(func() []string {
		var s []string
		for _, c := range bCmds {
			s = append(s, c.(string))
		}
		return s
	}(), "\n")

	if !strings.Contains(cmdStr, `install -Dm755 "lemond" "/app/bin/lemond"`) {
		t.Errorf("expected quoted binary install command with base name, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, `install -Dm644 "lemonade-app.desktop" "/app/share/applications/ai.lemonade_server.Lemonade.desktop"`) {
		t.Errorf("expected desktop destination in build-commands, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, `install -Dm644 "ai.lemonade_server.Lemonade.metainfo.xml" "/app/share/metainfo/ai.lemonade_server.Lemonade.metainfo.xml"`) {
		t.Errorf("expected metainfo destination in build-commands, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, `install -Dm644 "icons/128x128.png" "/app/share/icons/hicolor/128x128/apps/ai.lemonade_server.Lemonade.png"`) {
		t.Errorf("expected 128x128 icon in build-commands, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, `install -Dm644 "icons/scalable.svg" "/app/share/icons/hicolor/scalable/apps/ai.lemonade_server.Lemonade.svg"`) {
		t.Errorf("expected scalable icon in build-commands, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, `ln -sf "/app/bin/lemonade" "/app/bin/lemond"`) {
		t.Errorf("expected symlink in build-commands, got: %s", cmdStr)
	}

	// Also verify that ParseManifest can parse the generated manifest!
	manifestPath := filepath.Join(stateDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, yamlBytes, 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	parsed, err := ParseManifest(manifestPath)
	if err != nil {
		t.Fatalf("ParseManifest failed on generated manifest: %v", err)
	}
	if parsed.ID != "ai.lemonade_server.Lemonade" {
		t.Errorf("expected parsed ID ai.lemonade_server.Lemonade, got %q", parsed.ID)
	}
	if parsed.Runtime != "org.gnome.Platform" {
		t.Errorf("expected parsed Runtime org.gnome.Platform, got %q", parsed.Runtime)
	}
}

func TestGenerateManifestStagingDisambiguation(t *testing.T) {
	tempDir := t.TempDir()
	opts := GenerateOptions{
		AppID:   "com.example.CollisionApp",
		Runtime: "org.freedesktop.Platform",
		Binaries: []config.BinarySource{
			{Path: "client/tool", Dest: "/app/bin/tool-client"},
			{Path: "server/tool", Dest: "/app/bin/tool-server"},
		},
		Files: []config.FileSource{
			{Path: "assets/data.json", Dest: "/app/share/collision/data1.json"},
			{Path: "extra/data.json", Dest: "/app/share/collision/data2.json"},
		},
	}

	stateDir := filepath.Join(tempDir, ".state")
	_ = os.MkdirAll(stateDir, 0755)

	yamlBytes, err := GenerateManifestYAML(opts, stateDir)
	if err != nil {
		t.Fatalf("GenerateManifestYAML failed: %v", err)
	}

	var manifest map[string]interface{}
	if err := yaml.Unmarshal(yamlBytes, &manifest); err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}

	mods := manifest["modules"].([]interface{})
	mod0 := mods[0].(map[string]interface{})
	sources := mod0["sources"].([]interface{})

	// Check sources for unique dest-filenames
	stagedNames := make(map[string]bool)
	for _, src := range sources {
		sMap := src.(map[string]interface{})
		df := sMap["dest-filename"].(string)
		if stagedNames[df] {
			t.Errorf("duplicate staged dest-filename found: %q", df)
		}
		stagedNames[df] = true
	}

	if !stagedNames["tool"] || !stagedNames["tool_2"] {
		t.Errorf("expected tool and tool_2 in stagedNames, got: %+v", stagedNames)
	}
	if !stagedNames["data.json"] || !stagedNames["data_2.json"] {
		t.Errorf("expected data.json and data_2.json in stagedNames, got: %+v", stagedNames)
	}

	bCmds := mod0["build-commands"].([]interface{})
	cmdStr := fmt.Sprintf("%v", bCmds)
	if !strings.Contains(cmdStr, `install -Dm755 "tool" "/app/bin/tool-client"`) || !strings.Contains(cmdStr, `install -Dm755 "tool_2" "/app/bin/tool-server"`) {
		t.Errorf("expected build commands referencing disambiguated staged names, got: %s", cmdStr)
	}
}

func TestGenerateManifestDirectoryDotfiles(t *testing.T) {
	tempDir := t.TempDir()
	opts := GenerateOptions{
		AppID:   "com.example.DotfilesApp",
		Runtime: "org.freedesktop.Platform",
		Files: []config.FileSource{
			{Path: "my-assets/", Dest: "/app/share/dotfiles/assets"},
		},
	}

	stateDir := filepath.Join(tempDir, ".state")
	_ = os.MkdirAll(stateDir, 0755)

	yamlBytes, err := GenerateManifestYAML(opts, stateDir)
	if err != nil {
		t.Fatalf("GenerateManifestYAML failed: %v", err)
	}

	var manifest map[string]interface{}
	_ = yaml.Unmarshal(yamlBytes, &manifest)
	mods := manifest["modules"].([]interface{})
	mod0 := mods[0].(map[string]interface{})
	bCmds := mod0["build-commands"].([]interface{})
	cmdStr := fmt.Sprintf("%v", bCmds)

	if !strings.Contains(cmdStr, `cp -r "my-assets"/. "/app/share/dotfiles/assets"`) {
		t.Errorf("expected cp -r /. in build commands, got: %s", cmdStr)
	}
}
