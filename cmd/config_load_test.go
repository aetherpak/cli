package cmd

import (
	"os"
	"testing"
)

func TestLoadConfigYml(t *testing.T) {
	// Create temporary aetherpak.yml file in cwd
	data := []byte(`
registry: ghcr.io
pages_url: https://example.com
apps:
  - id: org.example.App
    manifest: apps/org.example.App.json
    runtime: org.gnome.Platform
`)
	err := os.WriteFile("aetherpak.yml", data, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yml")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Registry != "ghcr.io" {
		t.Errorf("expected registry ghcr.io, got %s", cfg.Registry)
	}
}

func TestLoadConfigNoSign(t *testing.T) {
	data := []byte(`
registry: ghcr.io
pages_url: https://example.com
no_sign: true
apps:
  - id: org.example.App
    manifest: apps/org.example.App.json
    runtime: org.gnome.Platform
`)
	err := os.WriteFile("aetherpak.yml", data, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	defer os.Remove("aetherpak.yml")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !cfg.NoSign {
		t.Errorf("expected no_sign true, got false")
	}
}
