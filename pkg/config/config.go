package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	appIDRegexp  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)
	branchRegexp = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	sha256Regexp = regexp.MustCompile(`^[a-f0-9]{64}$`)
	urlRegexp    = regexp.MustCompile(`^https?://`)

	supportedArches = map[string]bool{
		"x86_64":  true,
		"aarch64": true,
	}
)

// Config represents the aetherpak.yaml configuration file structure.
type Config struct {
	Registry        string            `yaml:"registry" json:"registry" mapstructure:"registry"`
	PagesURL        string            `yaml:"pages_url" json:"pages_url" mapstructure:"pages_url"`
	RemoteName      string            `yaml:"remote_name" json:"remote_name" mapstructure:"remote_name"`
	SigningMode     string            `yaml:"signing_mode" json:"signing_mode" mapstructure:"signing_mode"`
	RepoTitle       string            `yaml:"repo_title" json:"repo_title" mapstructure:"repo_title"`
	RepoHomepage    string            `yaml:"repo_homepage" json:"repo_homepage" mapstructure:"repo_homepage"`
	RuntimeRepo     string            `yaml:"runtime_repo" json:"runtime_repo" mapstructure:"runtime_repo"`
	Apps            []App             `yaml:"apps" json:"apps" mapstructure:"apps"`
	Linter          *LinterConfig     `yaml:"linter,omitempty" json:"linter,omitempty" mapstructure:"linter"`
	Branding        *BrandingConfig   `yaml:"branding,omitempty" json:"branding,omitempty" mapstructure:"branding"`
	Defaults        *DefaultsConfig   `yaml:"defaults,omitempty" json:"defaults,omitempty" mapstructure:"defaults"`
	ChannelMappings map[string]string `yaml:"channel_mappings,omitempty" json:"channel_mappings,omitempty" mapstructure:"channel_mappings"`
}

// LinterConfig defines validation linter strictness and rules to ignore.
type LinterConfig struct {
	Strict      *bool    `yaml:"strict" json:"strict" mapstructure:"strict"`
	IgnoreRules []string `yaml:"ignore_rules" json:"ignore_rules" mapstructure:"ignore_rules"`
}

// BrandingConfig defines custom landing page branding metadata.
type BrandingConfig struct {
	LogoURL       string `yaml:"logo_url" json:"logo_url" mapstructure:"logo_url"`
	FaviconURL    string `yaml:"favicon_url" json:"favicon_url" mapstructure:"favicon_url"`
	AccentColor   string `yaml:"accent_color" json:"accent_color" mapstructure:"accent_color"`
	FooterText    string `yaml:"footer_text" json:"footer_text" mapstructure:"footer_text"`
	IndexTemplate string `yaml:"index_template" json:"index_template" mapstructure:"index_template"`
}

// DefaultsConfig defines global repository build defaults.
type DefaultsConfig struct {
	CCache    *bool  `yaml:"ccache" json:"ccache" mapstructure:"ccache"`
	CCacheDir string `yaml:"ccache_dir" json:"ccache_dir" mapstructure:"ccache_dir"`
	StateDir  string `yaml:"state_dir" json:"state_dir" mapstructure:"state_dir"`
	RunLinter bool   `yaml:"run_linter" json:"run_linter" mapstructure:"run_linter"`
}

// App represents an individual application configuration.
type App struct {
	ID        string            `yaml:"id" json:"id" mapstructure:"id"`
	Branch    string            `yaml:"branch" json:"branch" mapstructure:"branch"`
	Arches    []string          `yaml:"arches" json:"arches" mapstructure:"arches"`
	Manifest  string            `yaml:"manifest,omitempty" json:"manifest,omitempty" mapstructure:"manifest"`
	Runtime   string            `yaml:"runtime,omitempty" json:"runtime,omitempty" mapstructure:"runtime"`
	RunLinter bool              `yaml:"run-linter" json:"run-linter" mapstructure:"run-linter"`
	Linter    *LinterConfig     `yaml:"linter,omitempty" json:"linter,omitempty" mapstructure:"linter"`
	CCache    *bool             `yaml:"ccache,omitempty" json:"ccache,omitempty" mapstructure:"ccache"`
	CCacheDir string            `yaml:"ccache_dir,omitempty" json:"ccache_dir,omitempty" mapstructure:"ccache_dir"`
	StateDir  string            `yaml:"state_dir,omitempty" json:"state_dir,omitempty" mapstructure:"state_dir"`
	Bundles   map[string]Bundle `yaml:"bundles,omitempty" json:"bundles,omitempty" mapstructure:"bundles"`
}

// Bundle represents an architecture-specific prebuilt flatpak bundle config.
type Bundle struct {
	URL    string `yaml:"url" json:"url" mapstructure:"url"`
	SHA256 string `yaml:"sha256" json:"sha256" mapstructure:"sha256"`
}

// Normalize sets default values for config and app fields.
func (cfg *Config) Normalize() {
	if cfg.Defaults == nil {
		cfg.Defaults = &DefaultsConfig{}
	}

	if cfg.Linter != nil {
		if cfg.Linter.Strict == nil {
			t := true
			cfg.Linter.Strict = &t
		}
	}

	for i := range cfg.Apps {
		app := &cfg.Apps[i]
		app.Normalize()

		if app.Linter == nil && cfg.Linter != nil {
			var rules []string
			if cfg.Linter.IgnoreRules != nil {
				rules = make([]string, len(cfg.Linter.IgnoreRules))
				copy(rules, cfg.Linter.IgnoreRules)
			}
			strictVal := *cfg.Linter.Strict
			app.Linter = &LinterConfig{
				Strict:      &strictVal,
				IgnoreRules: rules,
			}
		} else if app.Linter != nil {
			if app.Linter.Strict == nil {
				t := true
				app.Linter.Strict = &t
			}
		}

		if !app.RunLinter && cfg.Defaults.RunLinter {
			app.RunLinter = true
		}

		if app.CCache == nil && cfg.Defaults.CCache != nil {
			c := *cfg.Defaults.CCache
			app.CCache = &c
		}

		if app.CCacheDir == "" {
			if cfg.Defaults.CCacheDir != "" {
				app.CCacheDir = cfg.Defaults.CCacheDir
			} else {
				app.CCacheDir = ".ccache"
			}
		}

		if app.StateDir == "" {
			if cfg.Defaults.StateDir != "" {
				app.StateDir = cfg.Defaults.StateDir
			} else {
				app.StateDir = ".state"
			}
		}
	}
}

// Normalize sets default values for App fields if they are missing.
func (app *App) Normalize() {
	if app.Branch == "" {
		app.Branch = "stable"
	}
	if app.Manifest != "" {
		if len(app.Arches) == 0 {
			app.Arches = []string{"x86_64"}
		}
	}
}

// Validate asserts that the App configuration is structurally correct.
func (app *App) Validate() error {
	if app.ID == "" {
		return fmt.Errorf("app entry missing 'id'")
	}
	if !appIDRegexp.MatchString(app.ID) {
		return fmt.Errorf("app %q: 'id' must match format %s", app.ID, appIDRegexp.String())
	}

	branch := app.Branch
	if branch == "" {
		branch = "stable"
	}
	if !branchRegexp.MatchString(branch) {
		return fmt.Errorf("app %q: 'branch' must match format %s", app.ID, branchRegexp.String())
	}

	hasManifest := app.Manifest != ""
	hasBundles := len(app.Bundles) > 0

	if hasManifest == hasBundles {
		return fmt.Errorf("app %q: exactly one of 'manifest' or 'bundles' is required", app.ID)
	}

	if hasManifest {
		if strings.HasPrefix(app.Manifest, "/") {
			return fmt.Errorf("app %q: 'manifest' must be a relative path, cannot be absolute", app.ID)
		}
		// Check for traversal segments (e.g., ..)
		cleanPath := filepath.Clean(app.Manifest)
		if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
			return fmt.Errorf("app %q: 'manifest' must be a relative path with no '..' segments", app.ID)
		}
		if app.Runtime == "" {
			return fmt.Errorf("app %q: 'runtime' is required when 'manifest' is set", app.ID)
		}

		arches := app.Arches
		if len(arches) == 0 {
			arches = []string{"x86_64"}
		}
		for _, arch := range arches {
			if !supportedArches[arch] {
				return fmt.Errorf("app %q: unsupported arch %q", app.ID, arch)
			}
		}
	} else {
		for arch, b := range app.Bundles {
			if !supportedArches[arch] {
				return fmt.Errorf("app %q: unsupported bundle arch %q", app.ID, arch)
			}
			if b.URL == "" || b.SHA256 == "" {
				return fmt.Errorf("app %q bundle %q: 'url' and 'sha256' are required", app.ID, arch)
			}
			if !urlRegexp.MatchString(b.URL) {
				return fmt.Errorf("app %q bundle %q: 'url' must start with http:// or https://", app.ID, arch)
			}
			if !sha256Regexp.MatchString(b.SHA256) {
				return fmt.Errorf("app %q bundle %q: 'sha256' must be 64 lowercase hex characters", app.ID, arch)
			}
		}
	}

	return nil
}
