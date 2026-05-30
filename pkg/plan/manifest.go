package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// FlatpakManifest represents the metadata extracted from a Flatpak manifest.
type FlatpakManifest struct {
	ID             string
	Runtime        string
	RuntimeVersion string
}

// flatpakManifestRaw represents the raw JSON/YAML structure of a Flatpak manifest.
type flatpakManifestRaw struct {
	ID             string `json:"id" yaml:"id"`
	AppID          string `json:"app-id" yaml:"app-id"`
	Runtime        string `json:"runtime" yaml:"runtime"`
	RuntimeVersion string `json:"runtime-version" yaml:"runtime-version"`
}

// ParseManifest parses a Flatpak manifest file (JSON or YAML) and extracts key metadata.
func ParseManifest(path string) (*FlatpakManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	var m flatpakManifestRaw
	// Attempt JSON unmarshal first, fall back to YAML
	if err := json.Unmarshal(data, &m); err != nil {
		if yamlErr := yaml.Unmarshal(data, &m); yamlErr != nil {
			return nil, fmt.Errorf("failed to parse manifest as JSON or YAML: %w (yaml: %v)", err, yamlErr)
		}
	}

	// Resolve the application ID
	id := m.ID
	if id == "" {
		id = m.AppID
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("manifest is missing 'id' or 'app-id'")
	}

	runtime := strings.TrimSpace(m.Runtime)
	if runtime == "" {
		return nil, fmt.Errorf("manifest is missing 'runtime'")
	}

	runtimeVersion := strings.TrimSpace(m.RuntimeVersion)

	return &FlatpakManifest{
		ID:             id,
		Runtime:        runtime,
		RuntimeVersion: runtimeVersion,
	}, nil
}
