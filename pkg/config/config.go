package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

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

	// Top-level Single App / Zero-Manifest fields:
	AppID               string            `yaml:"app_id,omitempty" json:"app_id,omitempty" mapstructure:"app_id"`
	ID                  string            `yaml:"id,omitempty" json:"id,omitempty" mapstructure:"id"`
	Branch              string            `yaml:"branch,omitempty" json:"branch,omitempty" mapstructure:"branch"`
	Arches              []string          `yaml:"arches,omitempty" json:"arches,omitempty" mapstructure:"arches"`
	Manifest            string            `yaml:"manifest,omitempty" json:"manifest,omitempty" mapstructure:"manifest"`
	Runtime             string            `yaml:"runtime,omitempty" json:"runtime,omitempty" mapstructure:"runtime"`
	RuntimeVersion      string            `yaml:"runtime_version,omitempty" json:"runtime_version,omitempty" mapstructure:"runtime_version"`
	RuntimeVersionKebab string            `yaml:"runtime-version,omitempty" json:"-" mapstructure:"runtime-version"`
	SDK                 string            `yaml:"sdk,omitempty" json:"sdk,omitempty" mapstructure:"sdk"`
	SDKVersion          string            `yaml:"sdk_version,omitempty" json:"sdk_version,omitempty" mapstructure:"sdk_version"`
	SDKVersionKebab     string            `yaml:"sdk-version,omitempty" json:"-" mapstructure:"sdk-version"`
	Command             string            `yaml:"command,omitempty" json:"command,omitempty" mapstructure:"command"`
	FinishArgs          []string          `yaml:"finish_args,omitempty" json:"finish_args,omitempty" mapstructure:"finish_args"`
	FinishArgsKebab     []string          `yaml:"finish-args,omitempty" json:"-" mapstructure:"finish-args"`
	Permissions         []string          `yaml:"permissions,omitempty" json:"permissions,omitempty" mapstructure:"permissions"`
	Sources             *SourcesConfig    `yaml:"sources,omitempty" json:"sources,omitempty" mapstructure:"sources"`
	Bundles             map[string]Bundle `yaml:"bundles,omitempty" json:"bundles,omitempty" mapstructure:"bundles"`
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
	CCache        *bool                   `yaml:"ccache" json:"ccache" mapstructure:"ccache"`
	CCacheDir     string                  `yaml:"ccache_dir" json:"ccache_dir" mapstructure:"ccache_dir"`
	StateDir      string                  `yaml:"state_dir" json:"state_dir" mapstructure:"state_dir"`
	RunLinter     bool                    `yaml:"run_linter" json:"run_linter" mapstructure:"run_linter"`
	BuilderArgs   []string                `yaml:"builder_args,omitempty" json:"builder_args,omitempty" mapstructure:"builder_args"`
	Remotes       map[string]RemoteConfig `yaml:"remotes,omitempty" json:"remotes,omitempty" mapstructure:"remotes"`
	Flatpaks      []FlatpakDep            `yaml:"flatpaks,omitempty" json:"flatpaks,omitempty" mapstructure:"flatpaks"`
	NoInstallDeps *bool                   `yaml:"no_install_deps,omitempty" json:"no_install_deps,omitempty" mapstructure:"no_install_deps"`
	NoFlathub     *bool                   `yaml:"no_flathub,omitempty" json:"no_flathub,omitempty" mapstructure:"no_flathub"`
}

// SourcesConfig defines zero-manifest precompiled binary and artifact packaging.
type SourcesConfig struct {
	Binaries      []BinarySource `yaml:"binaries,omitempty" json:"binaries,omitempty" mapstructure:"binaries"`
	Desktop       string         `yaml:"desktop,omitempty" json:"desktop,omitempty" mapstructure:"desktop"`
	Metainfo      string         `yaml:"metainfo,omitempty" json:"metainfo,omitempty" mapstructure:"metainfo"`
	Appdata       string         `yaml:"appdata,omitempty" json:"appdata,omitempty" mapstructure:"appdata"`
	Icons         string         `yaml:"icons,omitempty" json:"icons,omitempty" mapstructure:"icons"`
	Files         []FileSource   `yaml:"files,omitempty" json:"files,omitempty" mapstructure:"files"`
	Symlinks      []string       `yaml:"symlinks,omitempty" json:"symlinks,omitempty" mapstructure:"symlinks"`
	BuildCommands []string       `yaml:"build_commands,omitempty" json:"build_commands,omitempty" mapstructure:"build_commands"`
	PostInstall   []string       `yaml:"post_install,omitempty" json:"post_install,omitempty" mapstructure:"post_install"`
}

// BinarySource represents a source binary to destination mapping.
type BinarySource struct {
	Path string `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`
	Dest string `yaml:"dest,omitempty" json:"dest,omitempty" mapstructure:"dest"`
	Src  string `yaml:"src,omitempty" json:"src,omitempty" mapstructure:"src"`
}

// FileSource represents a generic file or directory source to destination mapping.
type FileSource struct {
	Path string `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`
	Dest string `yaml:"dest,omitempty" json:"dest,omitempty" mapstructure:"dest"`
	Src  string `yaml:"src,omitempty" json:"src,omitempty" mapstructure:"src"`
}

// App represents an individual application configuration.
type App struct {
	ID                  string                  `yaml:"id" json:"id" mapstructure:"id"`
	AppID               string                  `yaml:"app_id,omitempty" json:"app_id,omitempty" mapstructure:"app_id"`
	Branch              string                  `yaml:"branch" json:"branch" mapstructure:"branch"`
	Arches              []string                `yaml:"arches" json:"arches" mapstructure:"arches"`
	Manifest            string                  `yaml:"manifest,omitempty" json:"manifest,omitempty" mapstructure:"manifest"`
	Runtime             string                  `yaml:"runtime,omitempty" json:"runtime,omitempty" mapstructure:"runtime"`
	RuntimeVersion      string                  `yaml:"runtime_version,omitempty" json:"runtime_version,omitempty" mapstructure:"runtime_version"`
	RuntimeVersionKebab string                  `yaml:"runtime-version,omitempty" json:"-" mapstructure:"runtime-version"`
	SDK                 string                  `yaml:"sdk,omitempty" json:"sdk,omitempty" mapstructure:"sdk"`
	SDKVersion          string                  `yaml:"sdk_version,omitempty" json:"sdk_version,omitempty" mapstructure:"sdk_version"`
	SDKVersionKebab     string                  `yaml:"sdk-version,omitempty" json:"-" mapstructure:"sdk-version"`
	Command             string                  `yaml:"command,omitempty" json:"command,omitempty" mapstructure:"command"`
	FinishArgs          []string                `yaml:"finish_args,omitempty" json:"finish_args,omitempty" mapstructure:"finish_args"`
	FinishArgsKebab     []string                `yaml:"finish-args,omitempty" json:"-" mapstructure:"finish-args"`
	Permissions         []string                `yaml:"permissions,omitempty" json:"permissions,omitempty" mapstructure:"permissions"`
	Sources             *SourcesConfig          `yaml:"sources,omitempty" json:"sources,omitempty" mapstructure:"sources"`
	RunLinter           bool                    `yaml:"run_linter" json:"run_linter" mapstructure:"run_linter"`
	RunLinterKebab      bool                    `yaml:"run-linter,omitempty" json:"-" mapstructure:"run-linter"`
	Linter              *LinterConfig           `yaml:"linter,omitempty" json:"linter,omitempty" mapstructure:"linter"`
	CCache              *bool                   `yaml:"ccache,omitempty" json:"ccache,omitempty" mapstructure:"ccache"`
	CCacheDir           string                  `yaml:"ccache_dir,omitempty" json:"ccache_dir,omitempty" mapstructure:"ccache_dir"`
	StateDir            string                  `yaml:"state_dir,omitempty" json:"state_dir,omitempty" mapstructure:"state_dir"`
	Bundles             map[string]Bundle       `yaml:"bundles,omitempty" json:"bundles,omitempty" mapstructure:"bundles"`
	BuilderArgs         []string                `yaml:"builder_args,omitempty" json:"builder_args,omitempty" mapstructure:"builder_args"`
	Remotes             map[string]RemoteConfig `yaml:"remotes,omitempty" json:"remotes,omitempty" mapstructure:"remotes"`
	Flatpaks            []FlatpakDep            `yaml:"flatpaks,omitempty" json:"flatpaks,omitempty" mapstructure:"flatpaks"`
	NoInstallDeps       *bool                   `yaml:"no_install_deps,omitempty" json:"no_install_deps,omitempty" mapstructure:"no_install_deps"`
	NoFlathub           *bool                   `yaml:"no_flathub,omitempty" json:"no_flathub,omitempty" mapstructure:"no_flathub"`
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

// UnmarshalYAML customizes YAML unmarshaling for SourcesConfig to support both lists and dictionary mappings.
func (s *SourcesConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node for sources")
	}

	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valNode := value.Content[i+1]

		switch keyNode.Value {
		case "binaries":
			if valNode.Kind == yaml.SequenceNode {
				var list []BinarySource
				if err := valNode.Decode(&list); err != nil {
					return err
				}
				s.Binaries = list
			} else if valNode.Kind == yaml.MappingNode {
				for j := 0; j < len(valNode.Content); j += 2 {
					k := valNode.Content[j].Value
					v := valNode.Content[j+1].Value
					s.Binaries = append(s.Binaries, BinarySource{Path: k, Src: k, Dest: v})
				}
			}
		case "desktop":
			s.Desktop = valNode.Value
		case "metainfo":
			s.Metainfo = valNode.Value
		case "appdata":
			s.Appdata = valNode.Value
		case "icons":
			s.Icons = valNode.Value
		case "files":
			if valNode.Kind == yaml.SequenceNode {
				var list []FileSource
				if err := valNode.Decode(&list); err != nil {
					return err
				}
				s.Files = list
			} else if valNode.Kind == yaml.MappingNode {
				for j := 0; j < len(valNode.Content); j += 2 {
					k := valNode.Content[j].Value
					v := valNode.Content[j+1].Value
					s.Files = append(s.Files, FileSource{Path: k, Src: k, Dest: v})
				}
			}
		case "symlinks":
			if valNode.Kind == yaml.SequenceNode {
				var list []string
				if err := valNode.Decode(&list); err != nil {
					return err
				}
				s.Symlinks = list
			} else if valNode.Kind == yaml.MappingNode {
				for j := 0; j < len(valNode.Content); j += 2 {
					k := valNode.Content[j].Value
					v := valNode.Content[j+1].Value
					s.Symlinks = append(s.Symlinks, fmt.Sprintf("%s: %s", k, v))
				}
			}
		case "build_commands", "build-commands":
			var list []string
			if err := valNode.Decode(&list); err != nil {
				return err
			}
			s.BuildCommands = list
		case "post_install", "post-install":
			var list []string
			if err := valNode.Decode(&list); err != nil {
				return err
			}
			s.PostInstall = list
		}
	}
	return nil
}

// UnmarshalYAML customizes YAML unmarshaling for BinarySource.
func (b *BinarySource) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		b.Path = s
		b.Src = s
		return nil
	}
	type rawBinarySource BinarySource
	var raw rawBinarySource
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*b = BinarySource(raw)
	if b.Path == "" && b.Src != "" {
		b.Path = b.Src
	}
	return nil
}

// UnmarshalJSON customizes JSON unmarshaling for BinarySource.
func (b *BinarySource) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		b.Path = s
		b.Src = s
		return nil
	}
	type rawBinarySource BinarySource
	var raw rawBinarySource
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*b = BinarySource(raw)
	if b.Path == "" && b.Src != "" {
		b.Path = b.Src
	}
	return nil
}

// String returns a friendly string representation of BinarySource.
func (b BinarySource) String() string {
	path := b.Path
	if path == "" {
		path = b.Src
	}
	if b.Dest != "" {
		return fmt.Sprintf("%s: %s", path, b.Dest)
	}
	return path
}

// UnmarshalYAML customizes YAML unmarshaling for FileSource.
func (f *FileSource) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		f.Path = s
		f.Src = s
		return nil
	}
	type rawFileSource FileSource
	var raw rawFileSource
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*f = FileSource(raw)
	if f.Path == "" && f.Src != "" {
		f.Path = f.Src
	}
	return nil
}

// UnmarshalJSON customizes JSON unmarshaling for FileSource.
func (f *FileSource) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		f.Path = s
		f.Src = s
		return nil
	}
	type rawFileSource FileSource
	var raw rawFileSource
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*f = FileSource(raw)
	if f.Path == "" && f.Src != "" {
		f.Path = f.Src
	}
	return nil
}

// String returns a friendly string representation of FileSource.
func (f FileSource) String() string {
	path := f.Path
	if path == "" {
		path = f.Src
	}
	if f.Dest != "" {
		return fmt.Sprintf("%s: %s", path, f.Dest)
	}
	return path
}

func flattenNestedMap(m map[string]interface{}, prefix string) (string, string) {
	for k, v := range m {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}
		if childMap, ok := v.(map[string]interface{}); ok {
			return flattenNestedMap(childMap, fullKey)
		}
		return fullKey, fmt.Sprintf("%v", v)
	}
	return "", ""
}

// SourcesConfigDecodeHook returns a mapstructure.DecodeHookFunc that handles dictionary maps for binaries and files.
func SourcesConfigDecodeHook() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if t != reflect.TypeOf(SourcesConfig{}) {
			return data, nil
		}
		m, ok := data.(map[string]interface{})
		if !ok {
			return data, nil
		}
		if bMap, ok := m["binaries"].(map[string]interface{}); ok {
			keys := make([]string, 0, len(bMap))
			for k := range bMap {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var binaries []BinarySource
			for _, k := range keys {
				v := bMap[k]
				if childMap, ok := v.(map[string]interface{}); ok {
					src, dest := flattenNestedMap(childMap, k)
					binaries = append(binaries, BinarySource{Path: src, Src: src, Dest: dest})
				} else {
					binaries = append(binaries, BinarySource{Path: k, Src: k, Dest: fmt.Sprintf("%v", v)})
				}
			}
			m["binaries"] = binaries
		}
		if fMap, ok := m["files"].(map[string]interface{}); ok {
			keys := make([]string, 0, len(fMap))
			for k := range fMap {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var files []FileSource
			for _, k := range keys {
				v := fMap[k]
				if childMap, ok := v.(map[string]interface{}); ok {
					src, dest := flattenNestedMap(childMap, k)
					files = append(files, FileSource{Path: src, Src: src, Dest: dest})
				} else {
					files = append(files, FileSource{Path: k, Src: k, Dest: fmt.Sprintf("%v", v)})
				}
			}
			m["files"] = files
		}
		return m, nil
	}
}

// BinarySourceDecodeHook returns a mapstructure.DecodeHookFunc for BinarySource.
func BinarySourceDecodeHook() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if t != reflect.TypeOf(BinarySource{}) {
			return data, nil
		}

		switch v := data.(type) {
		case string:
			return BinarySource{Path: v, Src: v}, nil
		case map[string]interface{}:
			var b BinarySource
			cfg := &mapstructure.DecoderConfig{
				Metadata: nil,
				Result:   &b,
				TagName:  "mapstructure",
			}
			decoder, err := mapstructure.NewDecoder(cfg)
			if err != nil {
				return nil, err
			}
			if err := decoder.Decode(v); err != nil {
				return nil, err
			}
			if b.Path == "" && b.Src != "" {
				b.Path = b.Src
			}
			return b, nil
		default:
			return data, nil
		}
	}
}

// FileSourceDecodeHook returns a mapstructure.DecodeHookFunc for FileSource.
func FileSourceDecodeHook() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if t != reflect.TypeOf(FileSource{}) {
			return data, nil
		}

		switch v := data.(type) {
		case string:
			return FileSource{Path: v, Src: v}, nil
		case map[string]interface{}:
			var fsrc FileSource
			cfg := &mapstructure.DecoderConfig{
				Metadata: nil,
				Result:   &fsrc,
				TagName:  "mapstructure",
			}
			decoder, err := mapstructure.NewDecoder(cfg)
			if err != nil {
				return nil, err
			}
			if err := decoder.Decode(v); err != nil {
				return nil, err
			}
			if fsrc.Path == "" && fsrc.Src != "" {
				fsrc.Path = fsrc.Src
			}
			return fsrc, nil
		default:
			return data, nil
		}
	}
}

// ParseRuntimeRef parses a runtime reference string like "org.gnome.Platform//49",
// "org.gnome.Platform/x86_64/49", or "org.gnome.Platform:49" into runtime and version.
func ParseRuntimeRef(ref string) (runtime, version string) {
	ref = strings.TrimSpace(ref)
	if strings.Contains(ref, "//") {
		parts := strings.SplitN(ref, "//", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if strings.Contains(ref, "/") {
		parts := strings.Split(ref, "/")
		if len(parts) >= 3 {
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[len(parts)-1])
		} else if len(parts) == 2 {
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		}
	}
	if strings.Contains(ref, ":") {
		parts := strings.SplitN(ref, ":", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return ref, ""
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

	// If no apps are configured, but top-level single app fields are specified,
	// synthesize cfg.Apps with the single app.
	if len(cfg.Apps) == 0 {
		appID := cfg.AppID
		if appID == "" {
			appID = cfg.ID
		}
		if appID != "" || cfg.Sources != nil || cfg.Manifest != "" || len(cfg.Bundles) > 0 {
			topApp := App{
				ID:                  appID,
				AppID:               appID,
				Branch:              cfg.Branch,
				Arches:              cfg.Arches,
				Manifest:            cfg.Manifest,
				Runtime:             cfg.Runtime,
				RuntimeVersion:      cfg.RuntimeVersion,
				RuntimeVersionKebab: cfg.RuntimeVersionKebab,
				SDK:                 cfg.SDK,
				SDKVersion:          cfg.SDKVersion,
				SDKVersionKebab:     cfg.SDKVersionKebab,
				Command:             cfg.Command,
				FinishArgs:          cfg.FinishArgs,
				FinishArgsKebab:     cfg.FinishArgsKebab,
				Permissions:         cfg.Permissions,
				Sources:             cfg.Sources,
				Bundles:             cfg.Bundles,
			}
			cfg.Apps = []App{topApp}
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

		if app.NoInstallDeps == nil && cfg.Defaults.NoInstallDeps != nil {
			val := *cfg.Defaults.NoInstallDeps
			app.NoInstallDeps = &val
		}

		if app.NoFlathub == nil && cfg.Defaults.NoFlathub != nil {
			val := *cfg.Defaults.NoFlathub
			app.NoFlathub = &val
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
	if app.ID == "" && app.AppID != "" {
		app.ID = app.AppID
	}
	if app.RunLinterKebab {
		app.RunLinter = true
	}
	if app.RuntimeVersion == "" && app.RuntimeVersionKebab != "" {
		app.RuntimeVersion = app.RuntimeVersionKebab
	}
	if app.SDKVersion == "" && app.SDKVersionKebab != "" {
		app.SDKVersion = app.SDKVersionKebab
	}
	if len(app.FinishArgs) == 0 && len(app.FinishArgsKebab) > 0 {
		app.FinishArgs = app.FinishArgsKebab
	}
	if len(app.FinishArgs) == 0 && len(app.Permissions) > 0 {
		app.FinishArgs = app.Permissions
	}

	if app.Runtime != "" {
		rt, ver := ParseRuntimeRef(app.Runtime)
		app.Runtime = rt
		if app.RuntimeVersion == "" && ver != "" {
			app.RuntimeVersion = ver
		}
	}

	if app.SDK == "" && app.Runtime != "" {
		if strings.Contains(app.Runtime, "Platform") {
			app.SDK = strings.Replace(app.Runtime, "Platform", "Sdk", 1)
		} else {
			app.SDK = "org.freedesktop.Sdk"
		}
	}

	if app.SDKVersion == "" && app.RuntimeVersion != "" {
		app.SDKVersion = app.RuntimeVersion
	}

	if app.Sources != nil {
		// Normalize binary sources
		for i := range app.Sources.Binaries {
			b := &app.Sources.Binaries[i]
			if b.Path == "" && b.Src != "" {
				b.Path = b.Src
			}
			if b.Dest == "" {
				b.Dest = "/app/bin/" + filepath.Base(b.Path)
			} else if strings.HasPrefix(b.Dest, "bin/") {
				b.Dest = "/app/" + b.Dest
			} else if !strings.HasPrefix(b.Dest, "/app/") {
				if !strings.HasPrefix(b.Dest, "/") {
					b.Dest = "/app/" + b.Dest
				}
			}
			b.Dest = filepath.Clean(b.Dest)
		}

		// Normalize file sources
		for i := range app.Sources.Files {
			f := &app.Sources.Files[i]
			if f.Path == "" && f.Src != "" {
				f.Path = f.Src
			}
			if f.Dest == "" && f.Path != "" {
				f.Dest = fmt.Sprintf("/app/share/%s/%s", app.ID, filepath.Base(strings.TrimSuffix(f.Path, "/")))
			}
			if f.Dest != "" && !strings.HasPrefix(f.Dest, "/app/") && !strings.HasPrefix(f.Dest, "/") {
				f.Dest = "/app/" + f.Dest
			}
			if f.Dest != "" {
				f.Dest = filepath.Clean(f.Dest)
			}
		}

		// Normalize desktop / metainfo / icons
		if app.Sources.Metainfo == "" && app.Sources.Appdata != "" {
			app.Sources.Metainfo = app.Sources.Appdata
		}

		// Infer command if missing
		if app.Command == "" {
			var candidateCommand string
			targetSuffix := ""
			if app.ID != "" {
				parts := strings.Split(app.ID, ".")
				targetSuffix = parts[len(parts)-1]
			}
			for _, b := range app.Sources.Binaries {
				dest := b.Dest
				if dest == "" {
					dest = b.Path
				}
				base := filepath.Base(dest)
				if targetSuffix != "" && base == targetSuffix {
					candidateCommand = base
					break
				}
			}
			if candidateCommand != "" {
				app.Command = candidateCommand
			} else if len(app.Sources.Binaries) > 0 {
				dest := app.Sources.Binaries[0].Dest
				if dest == "" {
					dest = app.Sources.Binaries[0].Path
				}
				app.Command = filepath.Base(dest)
			} else if targetSuffix != "" {
				app.Command = targetSuffix
			}
		}

		// Default GUI finish-args if none provided
		if len(app.FinishArgs) == 0 {
			app.FinishArgs = []string{
				"--socket=wayland",
				"--socket=fallback-x11",
				"--share=ipc",
				"--share=network",
				"--device=dri",
			}
		}
	}

	if app.Branch == "" {
		if app.Manifest != "" {
			if br := readManifestBranch(app.Manifest); br != "" {
				app.Branch = br
			}
		}
	}
	if app.Branch == "" {
		app.Branch = "stable"
	}
	if len(app.Arches) == 0 {
		app.Arches = []string{"x86_64"}
	}
}

func readManifestBranch(manifestPath string) string {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ""
	}
	var doc struct {
		Branch string `json:"branch" yaml:"branch"`
	}
	ext := strings.ToLower(filepath.Ext(manifestPath))
	if ext == ".json" {
		if err := json.Unmarshal(data, &doc); err == nil && doc.Branch != "" {
			return doc.Branch
		}
	} else if ext == ".yaml" || ext == ".yml" {
		if err := yaml.Unmarshal(data, &doc); err == nil && doc.Branch != "" {
			return doc.Branch
		}
	}
	return ""
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
	hasSources := app.Sources != nil

	count := 0
	if hasManifest {
		count++
	}
	if hasBundles {
		count++
	}
	if hasSources {
		count++
	}

	if count != 1 {
		return fmt.Errorf("app %q: exactly one of 'manifest', 'bundles', or 'sources' is required", app.ID)
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
	} else if hasBundles {
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
	} else if hasSources {
		if app.Runtime == "" {
			return fmt.Errorf("app %q: 'runtime' is required when using 'sources'", app.ID)
		}
		if len(app.Sources.Binaries) == 0 && app.Sources.Desktop == "" && len(app.Sources.Files) == 0 {
			return fmt.Errorf("app %q: 'sources' must declare at least one binary, desktop file, or file mapping", app.ID)
		}
		for _, b := range app.Sources.Binaries {
			path := b.Path
			if path == "" {
				path = b.Src
			}
			if path == "" {
				return fmt.Errorf("app %q: binary source path cannot be empty", app.ID)
			}
			if strings.HasPrefix(path, "/") {
				return fmt.Errorf("app %q: binary path %q must be a relative path, cannot be absolute", app.ID, path)
			}
			cleanPath := filepath.Clean(path)
			if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
				return fmt.Errorf("app %q: binary path %q cannot contain '..' segments", app.ID, path)
			}
			if b.Dest != "" && !strings.HasPrefix(b.Dest, "/app/") {
				return fmt.Errorf("app %q: binary destination %q must start with /app/", app.ID, b.Dest)
			}
		}
		if app.Sources.Desktop != "" {
			if strings.HasPrefix(app.Sources.Desktop, "/") {
				return fmt.Errorf("app %q: desktop path %q must be a relative path, cannot be absolute", app.ID, app.Sources.Desktop)
			}
			cleanPath := filepath.Clean(app.Sources.Desktop)
			if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
				return fmt.Errorf("app %q: desktop path %q cannot contain '..' segments", app.ID, app.Sources.Desktop)
			}
		}
		if app.Sources.Metainfo != "" {
			if strings.HasPrefix(app.Sources.Metainfo, "/") {
				return fmt.Errorf("app %q: metainfo path %q must be a relative path, cannot be absolute", app.ID, app.Sources.Metainfo)
			}
			cleanPath := filepath.Clean(app.Sources.Metainfo)
			if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
				return fmt.Errorf("app %q: metainfo path %q cannot contain '..' segments", app.ID, app.Sources.Metainfo)
			}
		}
		if app.Sources.Icons != "" {
			if strings.HasPrefix(app.Sources.Icons, "/") {
				return fmt.Errorf("app %q: icons path %q must be a relative path, cannot be absolute", app.ID, app.Sources.Icons)
			}
			cleanPath := filepath.Clean(app.Sources.Icons)
			if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
				return fmt.Errorf("app %q: icons path %q cannot contain '..' segments", app.ID, app.Sources.Icons)
			}
		}
		for _, f := range app.Sources.Files {
			path := f.Path
			if path == "" {
				path = f.Src
			}
			if path == "" {
				return fmt.Errorf("app %q: file source path cannot be empty", app.ID)
			}
			if strings.HasPrefix(path, "/") {
				return fmt.Errorf("app %q: file path %q must be a relative path, cannot be absolute", app.ID, path)
			}
			cleanPath := filepath.Clean(path)
			if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
				return fmt.Errorf("app %q: file path %q cannot contain '..' segments", app.ID, path)
			}
			if f.Dest == "" {
				return fmt.Errorf("app %q: generic file destination cannot be empty", app.ID)
			}
			if !strings.HasPrefix(f.Dest, "/app/") {
				return fmt.Errorf("app %q: file destination %q must start with /app/", app.ID, f.Dest)
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
		app.SDK != other.SDK || app.SDKVersion != other.SDKVersion ||
		app.Command != other.Command ||
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

	if !slicesEqual(app.FinishArgs, other.FinishArgs) {
		return false
	}

	if !sourcesEqual(app.Sources, other.Sources) {
		return false
	}

	if (app.CCache == nil) != (other.CCache == nil) {
		return false
	}
	if app.CCache != nil && *app.CCache != *other.CCache {
		return false
	}

	if (app.NoInstallDeps == nil) != (other.NoInstallDeps == nil) {
		return false
	}
	if app.NoInstallDeps != nil && *app.NoInstallDeps != *other.NoInstallDeps {
		return false
	}

	if (app.NoFlathub == nil) != (other.NoFlathub == nil) {
		return false
	}
	if app.NoFlathub != nil && *app.NoFlathub != *other.NoFlathub {
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

func sourcesEqual(a, b *SourcesConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Desktop != b.Desktop || a.Metainfo != b.Metainfo || a.Appdata != b.Appdata || a.Icons != b.Icons {
		return false
	}
	if len(a.Binaries) != len(b.Binaries) || len(a.Files) != len(b.Files) || len(a.Symlinks) != len(b.Symlinks) {
		return false
	}

	// Compare Binaries set-wise
	binCounts := make(map[BinarySource]int, len(a.Binaries))
	for _, bin := range a.Binaries {
		binCounts[bin]++
	}
	for _, bin := range b.Binaries {
		binCounts[bin]--
		if binCounts[bin] < 0 {
			return false
		}
	}

	// Compare Files set-wise
	fileCounts := make(map[FileSource]int, len(a.Files))
	for _, f := range a.Files {
		fileCounts[f]++
	}
	for _, f := range b.Files {
		fileCounts[f]--
		if fileCounts[f] < 0 {
			return false
		}
	}

	// Compare Symlinks set-wise
	symCounts := make(map[string]int, len(a.Symlinks))
	for _, s := range a.Symlinks {
		symCounts[s]++
	}
	for _, s := range b.Symlinks {
		symCounts[s]--
		if symCounts[s] < 0 {
			return false
		}
	}

	if !slicesEqual(a.BuildCommands, b.BuildCommands) || !slicesEqual(a.PostInstall, b.PostInstall) {
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
