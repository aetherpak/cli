package builder

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/executil"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/manifest"
	"github.com/aetherpak/aetherpak/pkg/repoinfo"
	"github.com/charmbracelet/lipgloss"
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
	Remotes              map[string]config.RemoteConfig // external Flatpak remotes to register
	Flatpaks             []config.FlatpakDep            // Flatpaks (runtimes, dependencies) to pre-install
	NoSign               bool                           // disable GPG verification for external remotes
	Install              bool                           // install application after build succeeds
	Bundle               bool                           // generate a bundled flatpak binary (.flatpak) for the application
	NoInstallDeps        bool                           // disable auto-injection of --install-deps-from flags
	NoFlathub            bool                           // disable auto-injection of flathub as a dependency remote
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

func getInstallationTarget(builderArgs []string) string {
	for _, arg := range builderArgs {
		if arg == "--user" || arg == "--system" || strings.HasPrefix(arg, "--installation=") {
			return arg
		}
	}
	if os.Getuid() == 0 {
		return "--system"
	}
	return "--user"
}

func hasInstallDepsFrom(args []string, remote string) bool {
	prefix := "--install-deps-from=" + remote
	for _, arg := range args {
		if arg == prefix {
			return true
		}
	}
	return false
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

	target := getInstallationTarget(opts.BuilderArgs)

	// Copy remotes and auto-register flathub if necessary
	remotes := make(map[string]config.RemoteConfig)
	for k, v := range opts.Remotes {
		remotes[k] = v
	}
	if !opts.NoFlathub {
		if _, exists := remotes["flathub"]; !exists {
			remotes["flathub"] = config.RemoteConfig{
				URL: "https://dl.flathub.org/repo/flathub.flatpakrepo",
			}
		}
	}

	// Pre-register flatpak remotes
	// Sort remotes for deterministic iteration/registration order
	var remoteNames []string
	for name := range remotes {
		remoteNames = append(remoteNames, name)
	}
	sort.Strings(remoteNames)

	for _, name := range remoteNames {
		r := remotes[name]
		logger.Info("Registering Flatpak remote %s: %s", name, r.URL)

		gpgVerify := (name == "flathub" || name == "flathub-beta")
		if r.GPGVerify != nil {
			gpgVerify = *r.GPGVerify
		} else if r.GPGKey != "" || r.SigVerifyURL != "" {
			gpgVerify = true
		}
		if opts.NoSign {
			gpgVerify = false
		}

		resolvedURL := r.URL
		if !gpgVerify {
			resolvedURL = resolveFlatpakrepoURL(r.URL)
		}

		// Prepare GPG key file if needed
		var gpgKeyFile string
		var cleanupGPGKey func()
		if gpgVerify && r.GPGKey != "" {
			var err error
			gpgKeyFile, cleanupGPGKey, err = prepareGPGKeyFile(r.GPGKey)
			if err != nil {
				return fmt.Errorf("failed to prepare GPG key for remote %s: %w", name, err)
			}
		}
		if cleanupGPGKey != nil {
			defer cleanupGPGKey()
		}

		cmdArgs := []string{"remote-add", target, "--if-not-exists"}
		if gpgVerify {
			if gpgKeyFile != "" {
				cmdArgs = append(cmdArgs, "--gpg-import="+gpgKeyFile)
			}
			if r.SigVerifyURL != "" {
				cmdArgs = append(cmdArgs, "--signature-lookaside="+r.SigVerifyURL)
			}
		} else {
			cmdArgs = append(cmdArgs, "--no-gpg-verify")
		}
		cmdArgs = append(cmdArgs, name, resolvedURL)

		if err := runFlatpakCommand(opts.Executor, cmdArgs); err != nil {
			return fmt.Errorf("failed to add flatpak remote %s (%s): %w", name, r.URL, err)
		}

		// Modify remote to ensure config settings are applied even if the remote already existed
		modifyArgs := []string{"remote-modify", target}
		if gpgVerify {
			modifyArgs = append(modifyArgs, "--gpg-verify")
			if gpgKeyFile != "" {
				modifyArgs = append(modifyArgs, "--gpg-import="+gpgKeyFile)
			}
			if r.SigVerifyURL != "" {
				modifyArgs = append(modifyArgs, "--signature-lookaside="+r.SigVerifyURL)
			}
		} else {
			modifyArgs = append(modifyArgs, "--no-gpg-verify")
		}
		modifyArgs = append(modifyArgs, name)

		if err := runFlatpakCommand(opts.Executor, modifyArgs); err != nil {
			logger.Info("WARNING: failed to modify flatpak remote %s: %v", name, err)
		}
	}

	// Pre-install flatpak dependencies
	for _, dep := range opts.Flatpaks {
		if dep.Remote == "" || dep.Ref == "" {
			continue
		}
		logger.Info("Installing Flatpak dependency %s from %s", dep.Ref, dep.Remote)
		if err := runFlatpakCommand(opts.Executor, []string{"install", target, "-y", dep.Remote, dep.Ref}); err != nil {
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

	if !opts.NoInstallDeps {
		var depRemotes []string
		for name := range remotes {
			depRemotes = append(depRemotes, name)
		}
		sort.Strings(depRemotes)
		for _, name := range depRemotes {
			if !hasInstallDepsFrom(opts.BuilderArgs, name) {
				args = append(args, "--install-deps-from="+name)
			}
		}
	}
	args = append(args, "--state-dir="+flatpakBuilderStateDir)

	if opts.CCacheDir != "" {
		args = append(args, "--ccache")
	}

	// Default to target installation for flatpak-builder to match the remote registration,
	// unless explicitly overridden in BuilderArgs.
	hasInstallLocation := false
	for _, arg := range opts.BuilderArgs {
		if arg == "--user" || arg == "--system" || strings.HasPrefix(arg, "--installation=") {
			hasInstallLocation = true
			break
		}
	}
	if !hasInstallLocation {
		args = append(args, target)
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

	var stdoutTargets []executil.StreamTarget
	var stderrTargets []executil.StreamTarget

	stdoutTargets = append(stdoutTargets, executil.StreamTarget{Writer: dest, Prefix: stdoutPrefix})
	stderrTargets = append(stderrTargets, executil.StreamTarget{Writer: dest, Prefix: stderrPrefix})

	if logger.HasLogFile() {
		stdoutTargets = append(stdoutTargets, executil.StreamTarget{Writer: logger.LogFileWriter(), Prefix: "flatpak-builder |"})
		stderrTargets = append(stderrTargets, executil.StreamTarget{Writer: logger.LogFileWriter(), Prefix: "flatpak-builder |"})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer stdoutPipe.Close()
		executil.StreamToTargets(stdoutPipe, stdoutTargets...)
	}()
	go func() {
		defer wg.Done()
		defer stderrPipe.Close()
		executil.StreamToTargets(stderrPipe, stderrTargets...)
	}()

	wg.Wait()
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

	mainAppID := opts.AppID
	if mainAppID == "" {
		if m, err := manifest.ParseManifest(opts.Manifest); err == nil {
			mainAppID = m.ID
		}
	}
	if mainAppID == "" {
		if info, err := repoinfo.Resolve(repoPath); err == nil {
			mainAppID = info.AppID
		}
	}
	if mainAppID == "" {
		return fmt.Errorf("failed to resolve application ID")
	}

	// Parse manifest to extract extension IDs
	var extensionIDs []string
	if m, err := manifest.ParseManifest(opts.Manifest); err == nil {
		extensionIDs = m.ExtensionIDs
	}

	if opts.Install {
		refs, err := repoinfo.ResolveAll(repoPath)
		if err != nil {
			// Fallback: assume only the main app ref
			refs = []repoinfo.Info{{
				AppID:    mainAppID,
				Branch:   opts.Branch,
				Arch:     opts.Arch,
				RepoPath: repoPath,
				RefType:  "app",
			}}
		}
		var relatedRefs []repoinfo.Info
		for _, ref := range refs {
			if manifest.IsRefRelated(ref.AppID, mainAppID, extensionIDs) {
				relatedRefs = append(relatedRefs, ref)
			}
		}
		if len(relatedRefs) == 0 {
			return fmt.Errorf("failed to install flatpak: no related refs found in repository")
		}
		absRepoPath, err := filepath.Abs(repoPath)
		if err != nil {
			absRepoPath = repoPath
		}
		for _, ref := range relatedRefs {
			logger.Info("Installing Flatpak ref %s (%s) from repo...", ref.AppID, ref.Ref())
			installArgs := []string{"install", target, "-y", "--or-update", absRepoPath, ref.AppID}
			if err := runFlatpakCommand(opts.Executor, installArgs); err != nil {
				return fmt.Errorf("failed to install flatpak ref %s: %w", ref.AppID, err)
			}
		}
	}

	if opts.Bundle {
		refs, err := repoinfo.ResolveAll(repoPath)
		if err != nil {
			// Fallback: assume only the main app ref
			refs = []repoinfo.Info{{
				AppID:    mainAppID,
				Branch:   opts.Branch,
				Arch:     opts.Arch,
				RepoPath: repoPath,
				RefType:  "app",
			}}
		}
		var relatedRefs []repoinfo.Info
		for _, ref := range refs {
			if manifest.IsRefRelated(ref.AppID, mainAppID, extensionIDs) {
				relatedRefs = append(relatedRefs, ref)
			}
		}
		if len(relatedRefs) == 0 {
			return fmt.Errorf("failed to generate flatpak bundle: no related refs found in repository")
		}
		for _, ref := range relatedRefs {
			bundleDir := filepath.Dir(repoPath)
			bundleFile := filepath.Join(bundleDir, ref.AppID+".flatpak")
			logger.Info("Generating Flatpak bundle: %s", bundleFile)

			bundleArgs := []string{
				"build-bundle",
			}
			if ref.RefType == "runtime" {
				bundleArgs = append(bundleArgs, "--runtime")
			}
			if ref.Arch != "" {
				bundleArgs = append(bundleArgs, "--arch="+ref.Arch)
			}
			bundleArgs = append(bundleArgs,
				repoPath,
				bundleFile,
				ref.AppID,
			)
			if ref.Branch != "" {
				bundleArgs = append(bundleArgs, ref.Branch)
			}

			if err := runFlatpakCommand(opts.Executor, bundleArgs); err != nil {
				return fmt.Errorf("failed to generate flatpak bundle for %s: %w", ref.AppID, err)
			}
		}
	}

	logger.Info("Build completed successfully for %s.", mainAppID)
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

	var targets []executil.StreamTarget
	targets = append(targets, executil.StreamTarget{Writer: dest, Prefix: prefix})
	if logger.HasLogFile() {
		targets = append(targets, executil.StreamTarget{Writer: logger.LogFileWriter(), Prefix: "flatpak-builder-lint |"})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer stdoutPipe.Close()
		executil.StreamToTargets(stdoutPipe, targets...)
	}()
	go func() {
		defer wg.Done()
		defer stderrPipe.Close()
		executil.StreamToTargets(stderrPipe, targets...)
	}()

	wg.Wait()
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

	var stdoutTargets []executil.StreamTarget
	var stderrTargets []executil.StreamTarget

	stdoutTargets = append(stdoutTargets, executil.StreamTarget{Writer: dest, Prefix: stdoutPrefix})
	stderrTargets = append(stderrTargets, executil.StreamTarget{Writer: dest, Prefix: stderrPrefix})

	if logger.HasLogFile() {
		stdoutTargets = append(stdoutTargets, executil.StreamTarget{Writer: logger.LogFileWriter(), Prefix: "flatpak |"})
		stderrTargets = append(stderrTargets, executil.StreamTarget{Writer: logger.LogFileWriter(), Prefix: "flatpak |"})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer stdoutPipe.Close()
		executil.StreamToTargets(stdoutPipe, stdoutTargets...)
	}()
	go func() {
		defer wg.Done()
		defer stderrPipe.Close()
		executil.StreamToTargets(stderrPipe, stderrTargets...)
	}()

	wg.Wait()
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

// resolveFlatpakrepoURL fetches the .flatpakrepo content and extracts the direct Url parameter.
// This allows registering OCI remotes directly to completely bypass keyring / GPG key imports.
func resolveFlatpakrepoURL(url string) string {
	if !strings.HasSuffix(url, ".flatpakrepo") {
		return url
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return url
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return url
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return url
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Url=") {
			return strings.TrimPrefix(line, "Url=")
		}
	}
	return url
}

// prepareGPGKeyFile resolves a GPG key (which can be a URL, an inline GPG key, or a local file path)
// and returns the local file path to the GPG key, along with a cleanup function if a temp file was created.
func prepareGPGKeyFile(key string) (string, func(), error) {
	if key == "" {
		return "", func() {}, nil
	}

	// 1. If it's inline GPG key
	if strings.Contains(key, "-----BEGIN PGP PUBLIC KEY BLOCK-----") {
		tmpFile, err := os.CreateTemp("", "aetherpak-gpg-key-*.asc")
		if err != nil {
			return "", nil, fmt.Errorf("failed to create temp file for GPG key: %w", err)
		}
		cleanup := func() { os.Remove(tmpFile.Name()) }
		if _, err := tmpFile.WriteString(key); err != nil {
			cleanup()
			tmpFile.Close()
			return "", nil, fmt.Errorf("failed to write GPG key to temp file: %w", err)
		}
		tmpFile.Close()
		return tmpFile.Name(), cleanup, nil
	}

	// 2. If it's a URL
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(key)
		if err != nil {
			return "", nil, fmt.Errorf("failed to download GPG key from URL %s: %w", key, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", nil, fmt.Errorf("failed to download GPG key from URL %s (status code %d)", key, resp.StatusCode)
		}
		tmpFile, err := os.CreateTemp("", "aetherpak-gpg-key-*.asc")
		if err != nil {
			return "", nil, fmt.Errorf("failed to create temp file for GPG key: %w", err)
		}
		cleanup := func() { os.Remove(tmpFile.Name()) }
		if _, err := io.Copy(tmpFile, resp.Body); err != nil {
			cleanup()
			tmpFile.Close()
			return "", nil, fmt.Errorf("failed to write downloaded GPG key to temp file: %w", err)
		}
		tmpFile.Close()
		return tmpFile.Name(), cleanup, nil
	}

	// 3. Otherwise, treat as local file path
	return key, func() {}, nil
}
