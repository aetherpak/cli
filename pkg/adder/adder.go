// Package adder orchestrates the `aetherpak add` workflow: resolve a source,
// build an App entry, edit the config (comment-preserving), show a diff, gate on
// confirmation, then write or roll back. All filesystem, git, and network IO is
// injected for testability; user-facing progress is reported via pkg/logger.
package adder

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/configdiff"
	"github.com/aetherpak/aetherpak/pkg/configedit"
	"github.com/aetherpak/aetherpak/pkg/gitutil"
	"github.com/aetherpak/aetherpak/pkg/importer"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/manifest"
)

// Source identifies which kind of app source is being added.
type Source int

const (
	SourceManifest Source = iota
	SourceBundle
	SourceGit
)

// ProgressFunc mirrors importer.ProgressFunc for dependency injection.
type ProgressFunc = importer.ProgressFunc

// Options configures a single add run.
type Options struct {
	ConfigPath string
	Source     Source

	ManifestPath string

	BundleURL    string
	BundleSHA256 string

	GitURL        string
	GitManifest   string
	SubmodulePath string

	// Overrides / defaults
	ID     string
	Branch string
	Arches []string

	// Options
	Toggles     map[string]bool // registry option Key -> enabled (see options.go)
	BuilderArgs []string        // free-form extra builder args (appended + deduped)

	Confirm bool // skip the diff gate
	Plain   bool // render the diff without ANSI styling

	// Injected dependencies (nil -> production defaults).
	Git      gitutil.Git
	Fetch    func(url string, p ProgressFunc) (string, string, error)
	Progress ProgressFunc
	In       io.Reader
	Out      io.Writer
	WorkDir  string // base dir for relative paths/rollback; "" -> cwd
	// PromptManifest, when set, is called for the git source if the manifest
	// cannot be auto-detected in the submodule; it returns a path relative to
	// the submodule root. When nil, an undetectable manifest is an error.
	PromptManifest func(repoDir string) (string, error)
}

func (o *Options) defaults() {
	if o.Git == nil {
		o.Git = gitutil.New()
	}
	if o.Fetch == nil {
		o.Fetch = importer.Fetch
	}
	if o.In == nil {
		o.In = os.Stdin
	}
	if o.Out == nil {
		o.Out = os.Stdout
	}
	if o.Branch == "" {
		o.Branch = "stable"
	}
	if len(o.Arches) == 0 {
		o.Arches = []string{DefaultArch()}
	}
}

// Run executes the add workflow.
func Run(opts Options) error {
	opts.defaults()

	if opts.WorkDir != "" {
		if opts.ConfigPath != "" && !filepath.IsAbs(opts.ConfigPath) {
			opts.ConfigPath = filepath.Join(opts.WorkDir, opts.ConfigPath)
		}
		if opts.ManifestPath != "" && !filepath.IsAbs(opts.ManifestPath) {
			opts.ManifestPath = filepath.Join(opts.WorkDir, opts.ManifestPath)
		}
	}

	app, cleanup, err := buildApp(&opts)
	if err != nil {
		return err
	}
	applyOptions(&app, opts.Source, opts.Toggles, opts.BuilderArgs)
	// cleanup undoes side effects (submodule add, temp downloads) when the
	// change is not committed.
	committed := false
	defer func() {
		if !committed && cleanup != nil {
			cleanup()
		}
	}()

	if err := app.Validate(); err != nil {
		return fmt.Errorf("resulting app entry is invalid: %w", err)
	}

	existing, err := os.ReadFile(opts.ConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read config %q: %w", opts.ConfigPath, err)
	}

	dup, err := configedit.HasApp(existing, app.ID)
	if err != nil {
		return err
	}
	if dup {
		return fmt.Errorf("app %q already exists in %s (editing existing apps is not supported)", app.ID, opts.ConfigPath)
	}

	updated, err := configedit.AppendApp(existing, app)
	if err != nil {
		return err
	}

	if !opts.Confirm {
		diff := configdiff.Unified(existing, updated, filepath.Base(opts.ConfigPath), opts.Plain)
		fmt.Fprint(opts.Out, diff) // diff already ends with a newline
		confirmed, err := confirm(opts.In, opts.Out)
		if err != nil {
			return fmt.Errorf("non-interactive environment detected: %w (use --confirm or -y)", err)
		}
		if !confirmed {
			logger.Info("Aborted; no changes written.")
			return nil // cleanup runs via defer
		}
	}

	if err := os.WriteFile(opts.ConfigPath, updated, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	committed = true
	logger.SuccessBanner("App Added", fmt.Sprintf("Added %s to %s.", app.ID, opts.ConfigPath))
	return nil
}

// buildApp resolves the source into a config.App and returns a cleanup func for
// any side effects that must be reverted if the change is not committed.
func buildApp(opts *Options) (config.App, func(), error) {
	switch opts.Source {
	case SourceManifest:
		return buildFromManifest(opts)
	case SourceBundle:
		return buildFromBundle(opts)
	case SourceGit:
		return buildFromGit(opts)
	default:
		return config.App{}, nil, fmt.Errorf("unknown source")
	}
}

func buildFromManifest(opts *Options) (config.App, func(), error) {
	if opts.ManifestPath == "" {
		return config.App{}, nil, fmt.Errorf("manifest path is required")
	}
	if _, err := os.Stat(opts.ManifestPath); err != nil {
		return config.App{}, nil, fmt.Errorf("manifest not found: %w", err)
	}
	id := opts.ID
	if id == "" {
		m, err := manifest.ParseManifest(opts.ManifestPath)
		if err != nil {
			return config.App{}, nil, fmt.Errorf("manifest %q did not contain a valid app id: %w", opts.ManifestPath, err)
		}
		id = m.ID
	}
	baseDir := filepath.Dir(opts.ConfigPath)
	if !filepath.IsAbs(baseDir) && opts.WorkDir != "" {
		baseDir = filepath.Join(opts.WorkDir, baseDir)
	}
	manifestPath := opts.ManifestPath
	if !filepath.IsAbs(manifestPath) && opts.WorkDir != "" {
		manifestPath = filepath.Join(opts.WorkDir, manifestPath)
	}
	rel, err := relativeTo(baseDir, manifestPath)
	if err != nil {
		return config.App{}, nil, fmt.Errorf("failed to resolve manifest path: %w", err)
	}
	return config.App{
		ID:       id,
		Branch:   opts.Branch,
		Arches:   opts.Arches,
		Manifest: rel,
	}, nil, nil
}

func buildFromBundle(opts *Options) (config.App, func(), error) {
	if opts.BundleURL == "" {
		return config.App{}, nil, fmt.Errorf("bundle URL is required")
	}
	if opts.ID == "" {
		return config.App{}, nil, fmt.Errorf("app id is required for bundle sources")
	}
	// A single .flatpak bundle contains binaries for one architecture only;
	// mapping it to several would be incorrect.
	if len(opts.Arches) > 1 {
		return config.App{}, nil, fmt.Errorf("bundle sources support a single architecture, but %d were given", len(opts.Arches))
	}
	tmpPath, sum, err := opts.Fetch(opts.BundleURL, opts.Progress)
	if err != nil {
		return config.App{}, nil, err
	}
	// The downloaded bundle is only needed to compute the checksum; `add` records
	// the URL + SHA-256 and never reuses the file, so release it unconditionally.
	if tmpPath != "" {
		defer os.Remove(tmpPath)
	}
	if opts.BundleSHA256 != "" && opts.BundleSHA256 != sum {
		return config.App{}, nil, fmt.Errorf("checksum mismatch: expected %s, got %s", opts.BundleSHA256, sum)
	}
	arch := opts.Arches[0]
	return config.App{
		ID:     opts.ID,
		Branch: opts.Branch,
		Bundles: map[string]config.Bundle{
			arch: {URL: opts.BundleURL, SHA256: sum},
		},
	}, nil, nil
}

func buildFromGit(opts *Options) (config.App, func(), error) {
	if opts.GitURL == "" {
		return config.App{}, nil, fmt.Errorf("git URL is required")
	}
	subPath := opts.SubmodulePath
	if subPath == "" {
		// The app id is only known after cloning, so the directory is named
		// after the repo (known from the URL up front).
		subPath = filepath.Join("manifests", repoName(opts.GitURL))
	}
	// Guard against traversal: subPath feeds both `git submodule add` and the
	// os.RemoveAll filesystem cleanup during rollback.
	if err := validateRelativeSubdir(subPath); err != nil {
		return config.App{}, nil, err
	}

	if err := opts.Git.SubmoduleAdd(opts.GitURL, subPath); err != nil {
		return config.App{}, nil, fmt.Errorf("failed to add submodule: %w", err)
	}
	cleanup := func() {
		if err := opts.Git.SubmoduleRemove(subPath); err != nil {
			logger.Warn("Failed to fully roll back submodule %q: %v (manual cleanup may be required)", subPath, err)
			return
		}
		logger.Info("Rolled back submodule %q. If .gitmodules or the index still show changes, run: git restore --staged .gitmodules && git checkout -- .gitmodules", subPath)
	}
	if err := opts.Git.SubmoduleUpdateInit(true); err != nil {
		cleanup()
		return config.App{}, nil, fmt.Errorf("failed to init submodule: %w", err)
	}

	subAbs := subPath
	if opts.WorkDir != "" {
		subAbs = filepath.Join(opts.WorkDir, subPath)
	}
	relManifest := opts.GitManifest
	if relManifest == "" {
		detected, err := manifest.DetectInDir(subAbs)
		if err != nil {
			// Only surface the manifest question when we genuinely cannot find
			// it; otherwise the missing manifest is a hard error.
			if opts.PromptManifest == nil {
				cleanup()
				return config.App{}, nil, fmt.Errorf("%w (pass --git-manifest to specify it)", err)
			}
			prompted, perr := opts.PromptManifest(subAbs)
			if perr != nil {
				cleanup()
				return config.App{}, nil, perr
			}
			relManifest = prompted
		} else {
			relManifest = detected
		}
	}

	id := opts.ID
	if id == "" {
		m, err := manifest.ParseManifest(filepath.Join(subAbs, relManifest))
		if err != nil {
			cleanup()
			return config.App{}, nil, fmt.Errorf("manifest %q did not contain a valid app id: %w", relManifest, err)
		}
		id = m.ID
	}

	baseDir := filepath.Dir(opts.ConfigPath)
	if !filepath.IsAbs(baseDir) && opts.WorkDir != "" {
		baseDir = filepath.Join(opts.WorkDir, baseDir)
	}
	manifestPath, err := relativeTo(baseDir, filepath.Join(subAbs, relManifest))
	if err != nil {
		cleanup()
		return config.App{}, nil, fmt.Errorf("failed to resolve manifest path: %w", err)
	}
	return config.App{
		ID:       id,
		Branch:   opts.Branch,
		Arches:   opts.Arches,
		Manifest: manifestPath,
	}, cleanup, nil
}

// DefaultArch returns the Flatpak architecture name for the host.
func DefaultArch() string { return archForGOARCH(runtime.GOARCH) }

func archForGOARCH(goarch string) string {
	if goarch == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}

// validateRelativeSubdir rejects submodule paths that are absolute or escape the
// working tree via "..", which would make filesystem cleanup delete unrelated
// directories.
func validateRelativeSubdir(p string) error {
	clean := filepath.Clean(p)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid submodule path %q: must be a relative subdirectory without '..'", p)
	}
	return nil
}

// relativeTo returns target expressed relative to baseDir, resolving both to
// absolute paths first so the inputs may be relative or absolute.
func relativeTo(baseDir, target string) (string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	return filepath.Rel(absBase, absTarget)
}

// repoName extracts a directory-friendly name from a git URL.
func repoName(url string) string {
	s := strings.TrimSuffix(url, ".git")
	s = strings.TrimRight(s, "/")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	if s == "" {
		return "app"
	}
	return s
}

// confirm reads a yes/no answer; default is no.
func confirm(in io.Reader, out io.Writer) (bool, error) {
	fmt.Fprint(out, "Apply these changes? [y/N]: ")
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return false, io.EOF
	}
	ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return ans == "y" || ans == "yes", nil
}
