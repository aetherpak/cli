// Package manifest extracts metadata from Flatpak manifest files (JSON or YAML)
// and locates manifests within a directory. It is shared by commands that need
// to read an app's id/runtime from its manifest.
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FlatpakManifest represents the metadata extracted from a Flatpak manifest.
type FlatpakManifest struct {
	ID             string
	Runtime        string
	RuntimeVersion string
	SDK            string
}

// flatpakManifestRaw represents the raw JSON/YAML structure of a Flatpak manifest.
type flatpakManifestRaw struct {
	ID             string `json:"id" yaml:"id"`
	AppID          string `json:"app-id" yaml:"app-id"`
	Runtime        string `json:"runtime" yaml:"runtime"`
	RuntimeVersion string `json:"runtime-version" yaml:"runtime-version"`
	SDK            string `json:"sdk" yaml:"sdk"`
}

// ParseManifest parses a Flatpak manifest file (JSON or YAML) and extracts key metadata.
func ParseManifest(path string) (*FlatpakManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	var m flatpakManifestRaw
	// Attempt JSON unmarshal first, fall back to YAML.
	if err := json.Unmarshal(data, &m); err != nil {
		if yamlErr := yaml.Unmarshal(data, &m); yamlErr != nil {
			return nil, fmt.Errorf("failed to parse manifest as JSON or YAML: %w (yaml: %v)", err, yamlErr)
		}
	}

	id := strings.TrimSpace(m.ID)
	if id == "" {
		id = strings.TrimSpace(m.AppID)
	}
	if id == "" {
		return nil, fmt.Errorf("manifest is missing 'id' or 'app-id'")
	}

	return &FlatpakManifest{
		ID:             id,
		Runtime:        strings.TrimSpace(m.Runtime),
		RuntimeVersion: strings.TrimSpace(m.RuntimeVersion),
		SDK:            strings.TrimSpace(m.SDK),
	}, nil
}

// manifestExts are the file extensions a Flatpak manifest may use.
var manifestExts = map[string]bool{".yml": true, ".yaml": true, ".json": true}

// DetectInDir finds the single Flatpak manifest file directly inside dir and
// returns its base name (relative to dir). It returns an error when there is no
// candidate or when the choice is ambiguous.
func DetectInDir(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("failed to read directory %q: %w", dir, err)
	}

	var candidates []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !manifestExts[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		// Require runtime/sdk so unrelated files with an 'id' field don't match.
		if m, err := ParseManifest(filepath.Join(dir, name)); err == nil && (m.Runtime != "" || m.SDK != "") {
			candidates = append(candidates, name)
		}
	}

	sort.Strings(candidates)
	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("no Flatpak manifest found in %q", dir)
	case 1:
		return candidates[0], nil
	default:
		return "", fmt.Errorf("multiple manifest candidates in %q: %s (specify --git-manifest)",
			dir, strings.Join(candidates, ", "))
	}
}
