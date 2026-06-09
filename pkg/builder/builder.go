package builder

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/executil"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

// BuildOptions contains options for executing flatpak-builder.
type BuildOptions struct {
	AppID                string
	Manifest             string
	Arch                 string
	Branch               string
	CCacheDir            string
	StateDir             string
	RepoPath             string
	RunLinter            bool
	LinterStrict         bool
	LinterIgnoreRules    []string
	LinterExceptions     []string // inline exceptions configured via CLI or config
	LinterExceptionsFile string   // path to linter exceptions configuration file (JSON)
	BuilderArgs          []string // extra flags passed through to flatpak-builder
	Executor             executil.Executor
	Remotes              map[string]string   // external Flatpak remotes to register
	Flatpaks             []config.FlatpakDep // Flatpaks (runtimes, dependencies) to pre-install
	NoSign               bool                // disable GPG verification for external remotes
}

// extraBuilderArgs appends a CI default to the pass-through flags: rofiles-fuse
// needs FUSE, absent in CI containers, so disable it under CI unless already set.
func extraBuilderArgs(passthrough []string, ciEnv string) []string {
	out := append([]string(nil), passthrough...)
	if ciEnv == "" {
		return out
	}
	for _, a := range out {
		if a == "--disable-rofiles-fuse" {
			return out
		}
	}
	return append(out, "--disable-rofiles-fuse")
}

// Build wraps the flatpak-builder execution.
func Build(opts BuildOptions) error {
	if opts.Executor == nil {
		opts.Executor = executil.NewOSExecutor()
	}
	logger.Info("Executing build for application: %s (arch: %s, branch: %s)", opts.AppID, opts.Arch, opts.Branch)

	if err := checkSubmodules(opts.Manifest); err != nil {
		return err
	}

	// Pre-register flatpak remotes
	for name, url := range opts.Remotes {
		logger.Info("Registering Flatpak remote %s: %s", name, url)
		cmdArgs := []string{"remote-add", "--user", "--if-not-exists"}
		if opts.NoSign || !strings.HasSuffix(url, ".flatpakrepo") {
			cmdArgs = append(cmdArgs, "--no-gpg-verify")
		}
		cmdArgs = append(cmdArgs, name, url)
		if err := runFlatpakCommand(opts.Executor, cmdArgs); err != nil {
			return fmt.Errorf("failed to add flatpak remote %s (%s): %w", name, url, err)
		}
		if opts.NoSign || !strings.HasSuffix(url, ".flatpakrepo") {
			// Ensure GPG verification is disabled even if the remote already existed
			_ = runFlatpakCommand(opts.Executor, []string{"remote-modify", "--user", "--no-gpg-verify", name})
		}
	}

	// Pre-install flatpak dependencies
	for _, dep := range opts.Flatpaks {
		if dep.Remote == "" || dep.Ref == "" {
			continue
		}
		logger.Info("Installing Flatpak dependency %s from %s", dep.Ref, dep.Remote)
		if err := runFlatpakCommand(opts.Executor, []string{"install", "--user", "-y", dep.Remote, dep.Ref}); err != nil {
			return fmt.Errorf("failed to install flatpak dependency %s from remote %s: %w", dep.Ref, dep.Remote, err)
		}
	}

	// Default linter ignore rules for AetherPak (since packages are self-hosted and not on Flathub,
	// mirroring screenshots to Flathub is not applicable).
	defaultIgnoreRules := []string{
		"appstream-external-screenshot-url",
		"appstream-screenshots-not-mirrored-in-ostree",
	}

	// Merge default ignore rules with user-provided ignore rules, ensuring no duplicates.
	ignoreRules := append([]string(nil), defaultIgnoreRules...)
	for _, r := range opts.LinterIgnoreRules {
		found := false
		for _, d := range defaultIgnoreRules {
			if r == d {
				found = true
				break
			}
		}
		if !found {
			ignoreRules = append(ignoreRules, r)
		}
	}

	// Merge user-provided exceptions, ensuring no duplicates.
	for _, r := range opts.LinterExceptions {
		found := false
		for _, existing := range ignoreRules {
			if r == existing {
				found = true
				break
			}
		}
		if !found {
			ignoreRules = append(ignoreRules, r)
		}
	}

	// Load exceptions from file if specified
	if opts.LinterExceptionsFile != "" {
		fileExceptions, err := loadExceptionsFile(opts.LinterExceptionsFile)
		if err != nil {
			return err
		}
		// Extract rules for this specific AppID and wildcard "*"
		var fileRules []string
		if opts.AppID != "" {
			if rules, ok := fileExceptions[opts.AppID]; ok {
				fileRules = append(fileRules, rules...)
			}
		}
		if rules, ok := fileExceptions["*"]; ok {
			fileRules = append(fileRules, rules...)
		}
		// Merge unique rules
		for _, r := range fileRules {
			found := false
			for _, existing := range ignoreRules {
				if r == existing {
					found = true
					break
				}
			}
			if !found {
				ignoreRules = append(ignoreRules, r)
			}
		}
	}

	var tempPath string
	if len(ignoreRules) > 0 {
		tempFile, err := os.CreateTemp(logger.TempDir(), "aetherpak-linter-*.json")
		if err != nil {
			return fmt.Errorf("failed to create temp file for linter exceptions: %w", err)
		}
		defer os.Remove(tempFile.Name())
		defer tempFile.Close()
		tempPath = tempFile.Name()

		appKey := opts.AppID
		if appKey == "" {
			appKey = "*"
		}
		exceptions := map[string][]string{
			appKey: ignoreRules,
		}
		if appKey != "*" {
			exceptions["*"] = ignoreRules
		}

		jsonData, err := json.Marshal(exceptions)
		if err != nil {
			return fmt.Errorf("failed to marshal linter exceptions: %w", err)
		}
		if _, err := tempFile.Write(jsonData); err != nil {
			return fmt.Errorf("failed to write linter exceptions: %w", err)
		}
		tempFile.Close()
	}

	if opts.RunLinter {
		var lintPrefix string
		if logger.IsPlain() {
			lintPrefix = "flatpak-builder-lint |"
		} else {
			lintPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true).Render("flatpak-builder-lint") + lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(" │")
		}
		logger.Info("Running flatpak-builder-lint for manifest: %s", opts.Manifest)
		lintArgs := []string{"manifest", opts.Manifest}
		if tempPath != "" {
			lintArgs = append(lintArgs, "--exceptions", "--user-exceptions", tempPath)
		}
		if err := runLinter(opts.Executor, lintArgs, lintPrefix); err != nil {
			if opts.LinterStrict {
				return fmt.Errorf("manifest linting failed: %w", err)
			}
			logger.Info("WARNING: manifest linting failed (non-strict mode): %v", err)
		}
	}

	// Ensure build directories are initialized
	stateDir := opts.StateDir
	if stateDir == "" {
		stateDir = ".state"
	}
	repoPath := opts.RepoPath
	if repoPath == "" {
		repoPath = "repo"
	}
	dirKey := opts.AppID
	if dirKey == "" {
		dirKey = strings.TrimSuffix(filepath.Base(opts.Manifest), filepath.Ext(opts.Manifest))
	}
	buildDir := filepath.Join(stateDir, "build-"+dirKey)
	flatpakBuilderStateDir := filepath.Join(stateDir, "state-"+dirKey)

	args := []string{
		"--force-clean",
		"--repo=" + repoPath,
	}
	if opts.Arch != "" {
		args = append(args, "--arch="+opts.Arch)
	}
	if opts.Branch != "" {
		args = append(args, "--default-branch="+opts.Branch)
	}
	args = append(args, "--state-dir="+flatpakBuilderStateDir)

	if opts.CCacheDir != "" {
		args = append(args, "--ccache")
	}

	// Default to --user installation for flatpak-builder to match the user remote registration,
	// unless explicitly overridden in BuilderArgs.
	hasInstallLocation := false
	for _, arg := range opts.BuilderArgs {
		if arg == "--user" || arg == "--system" || strings.HasPrefix(arg, "--installation=") {
			hasInstallLocation = true
			break
		}
	}
	if !hasInstallLocation {
		args = append(args, "--user")
	}

	args = append(args, extraBuilderArgs(opts.BuilderArgs, os.Getenv("CI"))...)

	// Append build directory and manifest file
	args = append(args, buildDir, opts.Manifest)

	logger.Debug("Running command: flatpak-builder %v", args)
	cmd := opts.Executor.Command("flatpak-builder", args...)

	var stdoutPrefix, stderrPrefix string
	if logger.IsPlain() {
		stdoutPrefix = "flatpak-builder |"
		stderrPrefix = "flatpak-builder |"
	} else {
		stdoutPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).Render("flatpak-builder") + lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(" │")
		stderrPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true).Render("flatpak-builder") + lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(" │")
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdoutPipe.Close()
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if opts.CCacheDir != "" {
		cmd.SetEnv(append(os.Environ(), "CCACHE_DIR="+opts.CCacheDir))
	}

	if err := cmd.Start(); err != nil {
		stdoutPipe.Close()
		stderrPipe.Close()
		return fmt.Errorf("failed to start flatpak-builder: %w", err)
	}

	var dest io.Writer = os.Stdout
	var lb *executil.LogBox
	if !logger.IsPlain() && isatty.IsTerminal(os.Stdout.Fd()) {
		lb = executil.NewLogBox(os.Stdout, 12).
			SetTitle(" flatpak-builder logs ").
			SetPrefixText("flatpak-builder").
			SetPrefixColor("99")
		lb.Start()
		dest = lb
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer stdoutPipe.Close()
		executil.StreamWithPrefix(stdoutPipe, dest, stdoutPrefix)
	}()
	go func() {
		defer wg.Done()
		defer stderrPipe.Close()
		executil.StreamWithPrefix(stderrPipe, dest, stderrPrefix)
	}()

	wg.Wait()
	if lb != nil {
		lb.Close()
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("flatpak-builder failed: %w", err)
	}

	if opts.RunLinter {
		var lintPrefix string
		if logger.IsPlain() {
			lintPrefix = "flatpak-builder-lint |"
		} else {
			lintPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true).Render("flatpak-builder-lint") + lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(" │")
		}
		logger.Info("Running flatpak-builder-lint for repository: %s", repoPath)
		lintArgs := []string{"repo", repoPath}
		if tempPath != "" {
			lintArgs = append(lintArgs, "--exceptions", "--user-exceptions", tempPath)
		}
		if err := runLinter(opts.Executor, lintArgs, lintPrefix); err != nil {
			if opts.LinterStrict {
				return fmt.Errorf("repository linting failed: %w", err)
			}
			logger.Info("WARNING: repository linting failed (non-strict mode): %v", err)
		}
	}

	logger.Info("Build completed successfully for %s.", opts.AppID)
	return nil
}

// checkSubmodules returns an error naming any uninitialized submodule under the
// manifest's directory, detected by reading .gitmodules rather than invoking git.
func checkSubmodules(manifest string) error {
	dir := filepath.Dir(manifest)
	if dir == "" {
		dir = "."
	}

	var uninitialized []string
	collectUninitializedSubmodules(dir, "", &uninitialized, 0)
	if len(uninitialized) > 0 {
		return fmt.Errorf(
			"uninitialized git submodule(s): %s — run 'git submodule update --init --recursive' before building",
			strings.Join(uninitialized, ", "),
		)
	}
	return nil
}

// collectUninitializedSubmodules records empty submodules from base/.gitmodules,
// recursing into populated ones. prefix is base relative to the start directory.
func collectUninitializedSubmodules(base, prefix string, out *[]string, depth int) {
	const maxDepth = 10
	if depth > maxDepth {
		return
	}
	data, err := os.ReadFile(filepath.Join(base, ".gitmodules"))
	if err != nil {
		return
	}
	for _, rel := range parseSubmodulePaths(string(data)) {
		path := filepath.Join(base, rel)
		display := filepath.Join(prefix, rel)
		if !isPopulated(path) {
			*out = append(*out, display)
			continue
		}
		collectUninitializedSubmodules(path, display, out, depth+1)
	}
}

// parseSubmodulePaths extracts the `path` values from .gitmodules content.
func parseSubmodulePaths(gitmodules string) []string {
	var paths []string
	for _, line := range strings.Split(gitmodules, "\n") {
		line = strings.TrimSpace(line)
		eq := strings.Index(line, "=")
		if eq < 0 || strings.TrimSpace(line[:eq]) != "path" {
			continue
		}
		if p := strings.TrimSpace(line[eq+1:]); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// isPopulated reports whether dir exists and is non-empty.
func isPopulated(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

func runLinter(executor executil.Executor, args []string, prefix string) error {
	cmdName, cmdArgs := resolveLinterCmd(executor)
	fullArgs := append(cmdArgs, args...)
	cmd := executor.Command(cmdName, fullArgs...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdoutPipe.Close()
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdoutPipe.Close()
		stderrPipe.Close()
		return fmt.Errorf("failed to start linter: %w", err)
	}

	var dest io.Writer = os.Stdout
	var lb *executil.LogBox
	if !logger.IsPlain() && isatty.IsTerminal(os.Stdout.Fd()) {
		lb = executil.NewLogBox(os.Stdout, 8).
			SetTitle(" flatpak-builder-lint logs ").
			SetPrefixText("flatpak-builder-lint").
			SetPrefixColor("13")
		lb.Start()
		dest = lb
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer stdoutPipe.Close()
		executil.StreamWithPrefix(stdoutPipe, dest, prefix)
	}()
	go func() {
		defer wg.Done()
		defer stderrPipe.Close()
		executil.StreamWithPrefix(stderrPipe, dest, prefix)
	}()

	wg.Wait()
	if lb != nil {
		lb.Close()
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func resolveLinterCmd(executor executil.Executor) (string, []string) {
	if _, err := executor.LookPath("flatpak-builder-lint"); err == nil {
		return "flatpak-builder-lint", nil
	}
	return "flatpak", []string{"run", "--command=flatpak-builder-lint", "org.flatpak.Builder"}
}

func loadExceptionsFile(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read linter exceptions file %q: %w", path, err)
	}
	var exceptions map[string][]string
	if err := json.Unmarshal(data, &exceptions); err != nil {
		return nil, fmt.Errorf("failed to parse linter exceptions file %q: %w", path, err)
	}
	return exceptions, nil
}

func runFlatpakCommand(executor executil.Executor, args []string) error {
	cmd := executor.Command("flatpak", args...)

	var stdoutPrefix, stderrPrefix string
	if logger.IsPlain() {
		stdoutPrefix = "flatpak |"
		stderrPrefix = "flatpak |"
	} else {
		stdoutPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Render("flatpak") + lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(" │")
		stderrPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true).Render("flatpak") + lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(" │")
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdoutPipe.Close()
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdoutPipe.Close()
		stderrPipe.Close()
		return fmt.Errorf("failed to start flatpak command: %w", err)
	}

	var dest io.Writer = os.Stdout
	var lb *executil.LogBox
	if !logger.IsPlain() && isatty.IsTerminal(os.Stdout.Fd()) {
		lb = executil.NewLogBox(os.Stdout, 8).
			SetTitle(" flatpak logs ").
			SetPrefixText("flatpak").
			SetPrefixColor("39")
		lb.Start()
		dest = lb
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer stdoutPipe.Close()
		executil.StreamWithPrefix(stdoutPipe, dest, stdoutPrefix)
	}()
	go func() {
		defer wg.Done()
		defer stderrPipe.Close()
		executil.StreamWithPrefix(stderrPipe, dest, stderrPrefix)
	}()

	wg.Wait()
	if lb != nil {
		lb.Close()
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}
