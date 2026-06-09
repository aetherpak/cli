package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/manifest"
	"github.com/go-viper/mapstructure/v2"
	"gopkg.in/yaml.v3"
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
	OCIRepository   string            `yaml:"oci_repository" json:"oci_repository" mapstructure:"oci_repository"`
	RemoteName      string            `yaml:"remote_name" json:"remote_name" mapstructure:"remote_name"`
	NoSign          bool              `yaml:"no_sign" json:"no_sign" mapstructure:"no_sign"`
	RepoTitle       string            `yaml:"repo_title" json:"repo_title" mapstructure:"repo_title"`
	RepoHomepage    string            `yaml:"repo_homepage" json:"repo_homepage" mapstructure:"repo_homepage"`
	RuntimeRepo     string            `yaml:"runtime_repo" json:"runtime_repo" mapstructure:"runtime_repo"`
	OutputDir       string            `yaml:"output_dir" json:"output_dir" mapstructure:"output_dir"`
	Apps            []App             `yaml:"apps" json:"apps" mapstructure:"apps"`
	Linter          *LinterConfig     `yaml:"linter,omitempty" json:"linter,omitempty" mapstructure:"linter"`
	Branding        *BrandingConfig   `yaml:"branding,omitempty" json:"branding,omitempty" mapstructure:"branding"`
	Defaults        *DefaultsConfig   `yaml:"defaults,omitempty" json:"defaults,omitempty" mapstructure:"defaults"`
	ChannelMappings map[string]string `yaml:"channel_mappings,omitempty" json:"channel_mappings,omitempty" mapstructure:"channel_mappings"`
}

// LinterConfig defines validation linter strictness and rules to ignore.
type LinterConfig struct {
	Strict         *bool    `yaml:"strict" json:"strict" mapstructure:"strict"`
	IgnoreRules    []string `yaml:"ignore_rules" json:"ignore_rules" mapstructure:"ignore_rules"`
	Exceptions     []string `yaml:"exceptions" json:"exceptions" mapstructure:"exceptions"`
	ExceptionsFile string   `yaml:"exceptions_file" json:"exceptions_file" mapstructure:"exceptions_file"`
}

// BrandingConfig defines custom landing page branding metadata.
type BrandingConfig struct {
	LogoURL       string `yaml:"logo_url" json:"logo_url" mapstructure:"logo_url"`
	FaviconURL    string `yaml:"favicon_url" json:"favicon_url" mapstructure:"favicon_url"`
	AccentColor   string `yaml:"accent_color" json:"accent_color" mapstructure:"accent_color"`
	FooterText    string `yaml:"footer_text" json:"footer_text" mapstructure:"footer_text"`
	IndexTemplate string `yaml:"index_template" json:"index_template" mapstructure:"index_template"`
}

// FlatpakDep represents an external Flatpak dependency (runtime, SDK extension, etc.) to be pre-installed.
type FlatpakDep struct {
	Remote string `yaml:"remote" json:"remote" mapstructure:"remote"`
	Ref    string `yaml:"ref" json:"ref" mapstructure:"ref"`
}

// DefaultsConfig defines global repository build defaults.
type DefaultsConfig struct {
	CCache      *bool                   `yaml:"ccache" json:"ccache" mapstructure:"ccache"`
	CCacheDir   string                  `yaml:"ccache_dir" json:"ccache_dir" mapstructure:"ccache_dir"`
	StateDir    string                  `yaml:"state_dir" json:"state_dir" mapstructure:"state_dir"`
	RunLinter   bool                    `yaml:"run_linter" json:"run_linter" mapstructure:"run_linter"`
	BuilderArgs []string                `yaml:"builder_args,omitempty" json:"builder_args,omitempty" mapstructure:"builder_args"`
	Remotes     map[string]RemoteConfig `yaml:"remotes,omitempty" json:"remotes,omitempty" mapstructure:"remotes"`
	Flatpaks    []FlatpakDep            `yaml:"flatpaks,omitempty" json:"flatpaks,omitempty" mapstructure:"flatpaks"`
}

// App represents an individual application configuration.
type App struct {
	ID       string   `yaml:"id" json:"id" mapstructure:"id"`
	Branch   string   `yaml:"branch" json:"branch" mapstructure:"branch"`
	Arches   []string `yaml:"arches" json:"arches" mapstructure:"arches"`
	Manifest string   `yaml:"manifest,omitempty" json:"manifest,omitempty" mapstructure:"manifest"`
	// Runtime is deprecated and is no longer required or used by the actions.
	Runtime string `yaml:"runtime,omitempty" json:"runtime,omitempty" mapstructure:"runtime"`
	// RuntimeVersion is deprecated and is no longer required or used by the actions.
	RuntimeVersion string                  `yaml:"runtime-version,omitempty" json:"runtime-version,omitempty" mapstructure:"runtime-version"`
	RunLinter      bool                    `yaml:"run_linter" json:"run_linter" mapstructure:"run_linter"`
	RunLinterKebab bool                    `yaml:"run-linter,omitempty" json:"-" mapstructure:"run-linter"`
	Linter         *LinterConfig           `yaml:"linter,omitempty" json:"linter,omitempty" mapstructure:"linter"`
	CCache         *bool                   `yaml:"ccache,omitempty" json:"ccache,omitempty" mapstructure:"ccache"`
	CCacheDir      string                  `yaml:"ccache_dir,omitempty" json:"ccache_dir,omitempty" mapstructure:"ccache_dir"`
	StateDir       string                  `yaml:"state_dir,omitempty" json:"state_dir,omitempty" mapstructure:"state_dir"`
	Bundles        map[string]Bundle       `yaml:"bundles,omitempty" json:"bundles,omitempty" mapstructure:"bundles"`
	BuilderArgs    []string                `yaml:"builder_args,omitempty" json:"builder_args,omitempty" mapstructure:"builder_args"`
	Remotes        map[string]RemoteConfig `yaml:"remotes,omitempty" json:"remotes,omitempty" mapstructure:"remotes"`
	Flatpaks       []FlatpakDep            `yaml:"flatpaks,omitempty" json:"flatpaks,omitempty" mapstructure:"flatpaks"`
}

// Bundle represents an architecture-specific prebuilt flatpak bundle config.
type Bundle struct {
	URL    string `yaml:"url" json:"url" mapstructure:"url"`
	SHA256 string `yaml:"sha256" json:"sha256" mapstructure:"sha256"`
}

// RemoteConfig represents a Flatpak remote configuration.
type RemoteConfig struct {
	URL          string `yaml:"url" json:"url" mapstructure:"url"`
	GPGVerify    *bool  `yaml:"gpg_verify,omitempty" json:"gpg_verify,omitempty" mapstructure:"gpg_verify"`
	GPGKey       string `yaml:"gpg_key,omitempty" json:"gpg_key,omitempty" mapstructure:"gpg_key"`
	SigVerifyURL string `yaml:"sig_verify_url,omitempty" json:"sig_verify_url,omitempty" mapstructure:"sig_verify_url"`
}

// UnmarshalYAML customizes YAML unmarshaling for RemoteConfig to handle both strings and map structures.
func (r *RemoteConfig) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		r.URL = s
		return nil
	}
	type rawRemoteConfig RemoteConfig
	var raw rawRemoteConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*r = RemoteConfig(raw)
	return nil
}

// UnmarshalJSON customizes JSON unmarshaling for RemoteConfig to handle both strings and object structures.
func (r *RemoteConfig) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		r.URL = s
		return nil
	}
	type rawRemoteConfig RemoteConfig
	var raw rawRemoteConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*r = RemoteConfig(raw)
	return nil
}

// Equal returns true if the RemoteConfig is structurally identical to another.
func (r RemoteConfig) Equal(other RemoteConfig) bool {
	if r.URL != other.URL || r.GPGKey != other.GPGKey || r.SigVerifyURL != other.SigVerifyURL {
		return false
	}
	if (r.GPGVerify == nil) != (other.GPGVerify == nil) {
		return false
	}
	if r.GPGVerify != nil && *r.GPGVerify != *other.GPGVerify {
		return false
	}
	return true
}

// String returns a friendly string representation of RemoteConfig.
func (r RemoteConfig) String() string {
	if r.GPGVerify == nil && r.GPGKey == "" && r.SigVerifyURL == "" {
		return r.URL
	}
	var parts []string
	parts = append(parts, "url="+r.URL)
	if r.GPGVerify != nil {
		parts = append(parts, fmt.Sprintf("gpg_verify=%v", *r.GPGVerify))
	}
	if r.GPGKey != "" {
		parts = append(parts, "gpg_key="+r.GPGKey)
	}
	if r.SigVerifyURL != "" {
		parts = append(parts, "sig_verify_url="+r.SigVerifyURL)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// RemoteConfigDecodeHook returns a mapstructure.DecodeHookFunc that decodes a string or a map into a RemoteConfig.
func RemoteConfigDecodeHook() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if t != reflect.TypeOf(RemoteConfig{}) {
			return data, nil
		}

		switch v := data.(type) {
		case string:
			return RemoteConfig{URL: v}, nil
		case map[string]interface{}:
			var r RemoteConfig
			cfg := &mapstructure.DecoderConfig{
				Metadata: nil,
				Result:   &r,
				TagName:  "mapstructure",
			}
			decoder, err := mapstructure.NewDecoder(cfg)
			if err != nil {
				return nil, err
			}
			if err := decoder.Decode(v); err != nil {
				return nil, err
			}
			return r, nil
		case map[interface{}]interface{}:
			m := make(map[string]interface{})
			for key, val := range v {
				m[fmt.Sprintf("%v", key)] = val
			}
			var r RemoteConfig
			cfg := &mapstructure.DecoderConfig{
				Metadata: nil,
				Result:   &r,
				TagName:  "mapstructure",
			}
			decoder, err := mapstructure.NewDecoder(cfg)
			if err != nil {
				return nil, err
			}
			if err := decoder.Decode(m); err != nil {
				return nil, err
			}
			return r, nil
		default:
			return data, nil
		}
	}
}

// Normalize sets default values for config and app fields.
func (cfg *Config) Normalize() {
	if cfg.OCIRepository == "" && cfg.RemoteName != "" {
		cfg.OCIRepository = cfg.RemoteName
	}

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
			var exceptions []string
			if cfg.Linter.Exceptions != nil {
				exceptions = make([]string, len(cfg.Linter.Exceptions))
				copy(exceptions, cfg.Linter.Exceptions)
			}
			strictVal := *cfg.Linter.Strict
			app.Linter = &LinterConfig{
				Strict:         &strictVal,
				IgnoreRules:    rules,
				Exceptions:     exceptions,
				ExceptionsFile: cfg.Linter.ExceptionsFile,
			}
		} else if app.Linter != nil {
			if app.Linter.Strict == nil {
				t := true
				app.Linter.Strict = &t
			}
			if app.Linter.ExceptionsFile == "" && cfg.Linter != nil {
				app.Linter.ExceptionsFile = cfg.Linter.ExceptionsFile
			}
			if cfg.Linter != nil {
				if len(cfg.Linter.IgnoreRules) > 0 {
					merged := append([]string(nil), app.Linter.IgnoreRules...)
					for _, r := range cfg.Linter.IgnoreRules {
						found := false
						for _, existing := range merged {
							if r == existing {
								found = true
								break
							}
						}
						if !found {
							merged = append(merged, r)
						}
					}
					app.Linter.IgnoreRules = merged
				}
				if len(cfg.Linter.Exceptions) > 0 {
					merged := append([]string(nil), app.Linter.Exceptions...)
					for _, ex := range cfg.Linter.Exceptions {
						found := false
						for _, existing := range merged {
							if ex == existing {
								found = true
								break
							}
						}
						if !found {
							merged = append(merged, ex)
						}
					}
					app.Linter.Exceptions = merged
				}
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
				if cfg.OutputDir != "" {
					app.CCacheDir = filepath.Join(cfg.OutputDir, ".ccache")
				} else {
					app.CCacheDir = ".ccache"
				}
			}
		}

		if app.StateDir == "" {
			if cfg.Defaults.StateDir != "" {
				app.StateDir = cfg.Defaults.StateDir
			} else {
				if cfg.OutputDir != "" {
					app.StateDir = filepath.Join(cfg.OutputDir, ".state")
				} else {
					app.StateDir = ".state"
				}
			}
		}

		if len(app.BuilderArgs) == 0 && len(cfg.Defaults.BuilderArgs) > 0 {
			app.BuilderArgs = make([]string, len(cfg.Defaults.BuilderArgs))
			copy(app.BuilderArgs, cfg.Defaults.BuilderArgs)
		}

		if len(app.Remotes) == 0 && len(cfg.Defaults.Remotes) > 0 {
			app.Remotes = make(map[string]RemoteConfig)
			for k, v := range cfg.Defaults.Remotes {
				app.Remotes[k] = v
			}
		} else if len(cfg.Defaults.Remotes) > 0 {
			merged := make(map[string]RemoteConfig)
			for k, v := range cfg.Defaults.Remotes {
				merged[k] = v
			}
			for k, v := range app.Remotes {
				merged[k] = v
			}
			app.Remotes = merged
		}

		if len(app.Flatpaks) == 0 && len(cfg.Defaults.Flatpaks) > 0 {
			app.Flatpaks = make([]FlatpakDep, len(cfg.Defaults.Flatpaks))
			copy(app.Flatpaks, cfg.Defaults.Flatpaks)
		} else if len(cfg.Defaults.Flatpaks) > 0 {
			merged := append([]FlatpakDep(nil), cfg.Defaults.Flatpaks...)
			for _, dep := range app.Flatpaks {
				exists := false
				for _, m := range merged {
					if m.Remote == dep.Remote && m.Ref == dep.Ref {
						exists = true
						break
					}
				}
				if !exists {
					merged = append(merged, dep)
				}
			}
			app.Flatpaks = merged
		}
	}
}

// Normalize sets default values for App fields if they are missing.
func (app *App) Normalize() {
	if app.RunLinterKebab {
		app.RunLinter = true
	}
	if app.Branch == "" {
		if app.Manifest != "" {
			if m, err := manifest.ParseManifest(app.Manifest); err == nil && m.Branch != "" {
				app.Branch = m.Branch
			}
		}
	}
	if app.Branch == "" {
		app.Branch = "stable"
	}
	if app.Manifest != "" {
		if len(app.Arches) == 0 {
			app.Arches = []string{"x86_64"}
		}
	}
}

// ValidateBasic validates basic metadata (ID, branch, runtime, and arches) without path checks.
func (app *App) ValidateBasic() error {
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

	for _, arch := range app.Arches {
		if !supportedArches[arch] {
			return fmt.Errorf("app %q: unsupported arch %q", app.ID, arch)
		}
	}
	return nil
}

// Validate asserts that the App configuration is structurally correct.
func (app *App) Validate() error {
	if err := app.ValidateBasic(); err != nil {
		return err
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

	for name, r := range app.Remotes {
		if name == "" {
			return fmt.Errorf("app %q: flatpak remote name cannot be empty", app.ID)
		}
		if r.URL == "" {
			return fmt.Errorf("app %q: flatpak remote %q URL cannot be empty", app.ID, name)
		}
		if !urlRegexp.MatchString(r.URL) {
			return fmt.Errorf("app %q: flatpak remote %q URL %q must start with http:// or https://", app.ID, name, r.URL)
		}
	}

	for _, dep := range app.Flatpaks {
		if dep.Remote == "" {
			return fmt.Errorf("app %q: flatpak dependency remote cannot be empty", app.ID)
		}
		if dep.Ref == "" {
			return fmt.Errorf("app %q: flatpak dependency ref cannot be empty", app.ID)
		}
	}

	return nil
}

// ValidateArch returns an error if the architecture is not supported.
// An empty string is considered valid.
func ValidateArch(arch string) error {
	if arch == "" {
		return nil
	}
	if !supportedArches[arch] {
		return fmt.Errorf("unsupported architecture %q (must be 'x86_64' or 'aarch64')", arch)
	}
	return nil
}

// Equal returns true if the App configuration is structurally identical to another App.
func (app App) Equal(other App) bool {
	if app.ID != other.ID || app.Branch != other.Branch || app.Manifest != other.Manifest ||
		app.Runtime != other.Runtime || app.RuntimeVersion != other.RuntimeVersion ||
		app.RunLinter != other.RunLinter ||
		app.CCacheDir != other.CCacheDir || app.StateDir != other.StateDir {
		return false
	}

	if !slicesEqual(app.Arches, other.Arches) {
		return false
	}

	if !slicesEqual(app.BuilderArgs, other.BuilderArgs) {
		return false
	}

	if (app.CCache == nil) != (other.CCache == nil) {
		return false
	}
	if app.CCache != nil && *app.CCache != *other.CCache {
		return false
	}

	if !linterConfigEqual(app.Linter, other.Linter) {
		return false
	}

	if len(app.Bundles) != len(other.Bundles) {
		return false
	}
	for k, v := range app.Bundles {
		ov, ok := other.Bundles[k]
		if !ok || v != ov {
			return false
		}
	}

	if !flatpakRemotesEqual(app.Remotes, other.Remotes) {
		return false
	}

	if !flatpakDepsEqual(app.Flatpaks, other.Flatpaks) {
		return false
	}

	return true
}

func flatpakRemotesEqual(a, b map[string]RemoteConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok || !v.Equal(bv) {
			return false
		}
	}
	return true
}

func flatpakDepsEqual(a, b []FlatpakDep) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Remote != b[i].Remote || a[i].Ref != b[i].Ref {
			return false
		}
	}
	return true
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func linterConfigEqual(a, b *LinterConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if (a.Strict == nil) != (b.Strict == nil) {
		return false
	}
	if a.Strict != nil && *a.Strict != *b.Strict {
		return false
	}
	if a.ExceptionsFile != b.ExceptionsFile {
		return false
	}
	return stringSlicesEqualAsSets(a.IgnoreRules, b.IgnoreRules) && stringSlicesEqualAsSets(a.Exceptions, b.Exceptions)
}

func stringSlicesEqualAsSets(a, b []string) bool {
	setA := make(map[string]bool)
	for _, x := range a {
		setA[x] = true
	}
	setB := make(map[string]bool)
	for _, x := range b {
		setB[x] = true
	}
	if len(setA) != len(setB) {
		return false
	}
	for k := range setA {
		if !setB[k] {
			return false
		}
	}
	return true
}
