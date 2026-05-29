package builder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/charmbracelet/lipgloss"
)

// BuildOptions contains options for executing flatpak-builder.
type BuildOptions struct {
	AppID             string
	Manifest          string
	Arch              string
	Branch            string
	CCacheDir         string
	StateDir          string
	RepoPath          string
	RunLinter         bool
	LinterStrict      bool
	LinterIgnoreRules []string
}

// Build wraps the flatpak-builder execution.
func Build(opts BuildOptions) error {
	logger.Info("Executing build for application: %s (arch: %s, branch: %s)", opts.AppID, opts.Arch, opts.Branch)

	var tempPath string
	if len(opts.LinterIgnoreRules) > 0 {
		tempFile, err := os.CreateTemp("", "aetherpak-linter-*.json")
		if err != nil {
			return fmt.Errorf("failed to create temp file for linter exceptions: %w", err)
		}
		defer os.Remove(tempFile.Name())
		tempPath = tempFile.Name()

		appKey := opts.AppID
		if appKey == "" {
			appKey = "*"
		}
		exceptions := map[string][]string{
			appKey: opts.LinterIgnoreRules,
		}
		if appKey != "*" {
			exceptions["*"] = opts.LinterIgnoreRules
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
		if err := runLinter(lintArgs, lintPrefix); err != nil {
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
		"--arch=" + opts.Arch,
		"--default-branch=" + opts.Branch,
		"--state-dir=" + flatpakBuilderStateDir,
	}

	if opts.CCacheDir != "" {
		args = append(args, "--ccache")
	}

	// Append build directory and manifest file
	args = append(args, buildDir, opts.Manifest)

	logger.Debug("Running command: flatpak-builder %v", args)
	cmd := exec.Command("flatpak-builder", args...)

	var stdoutPrefix, stderrPrefix string
	if logger.IsPlain() {
		stdoutPrefix = "flatpak-builder |"
		stderrPrefix = "flatpak-builder |"
	} else {
		stdoutPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).Render("flatpak-builder") + lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(" │")
		stderrPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true).Render("flatpak-builder") + lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(" │")
	}

	stdoutWriter := newPrefixedWriter(stdoutPrefix)
	stderrWriter := newPrefixedWriter(stderrPrefix)

	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	if opts.CCacheDir != "" {
		cmd.Env = append(os.Environ(), "CCACHE_DIR="+opts.CCacheDir)
	}

	if err := cmd.Run(); err != nil {
		stdoutWriter.Flush()
		stderrWriter.Flush()
		return fmt.Errorf("flatpak-builder failed: %w", err)
	}

	stdoutWriter.Flush()
	stderrWriter.Flush()

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
		if err := runLinter(lintArgs, lintPrefix); err != nil {
			if opts.LinterStrict {
				return fmt.Errorf("repository linting failed: %w", err)
			}
			logger.Info("WARNING: repository linting failed (non-strict mode): %v", err)
		}
	}

	logger.Info("Build completed successfully for %s.", opts.AppID)
	return nil
}

func runLinter(args []string, prefix string) error {
	cmdName, cmdArgs := resolveLinterCmd()
	fullArgs := append(cmdArgs, args...)
	cmd := exec.Command(cmdName, fullArgs...)
	writer := newPrefixedWriter(prefix)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		writer.Flush()
		return err
	}
	writer.Flush()
	return nil
}

func resolveLinterCmd() (string, []string) {
	if _, err := exec.LookPath("flatpak-builder-lint"); err == nil {
		return "flatpak-builder-lint", nil
	}
	return "flatpak", []string{"run", "--command=flatpak-builder-lint", "org.flatpak.Builder"}
}

type prefixedWriter struct {
	prefix string
	buffer []byte
}

func newPrefixedWriter(prefix string) *prefixedWriter {
	return &prefixedWriter{prefix: prefix}
}

func (w *prefixedWriter) Write(p []byte) (n int, err error) {
	w.buffer = append(w.buffer, p...)
	for {
		idx := bytes.IndexByte(w.buffer, '\n')
		if idx == -1 {
			break
		}
		line := string(w.buffer[:idx])
		w.buffer = w.buffer[idx+1:]
		line = strings.TrimSuffix(line, "\r")
		fmt.Fprintf(os.Stdout, "%s %s\n", w.prefix, line)
	}
	return len(p), nil
}

func (w *prefixedWriter) Flush() {
	if len(w.buffer) > 0 {
		line := string(w.buffer)
		line = strings.TrimSuffix(line, "\r")
		line = strings.TrimSuffix(line, "\n")
		fmt.Fprintf(os.Stdout, "%s %s\n", w.prefix, line)
		w.buffer = nil
	}
}
