// Package gitutil provides a small, reusable abstraction over git operations.
//
// The exec-backed ExecGit implementation shells out to the system git binary
// through pkg/executil (so it is mockable in tests). The Git interface is the
// seam intended to be re-implemented with a native Go git library later without
// touching callers.
package gitutil

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/executil"
)

// Git is the set of git operations used across the CLI.
type Git interface {
	// Toplevel returns the absolute path of the working tree root.
	Toplevel() (string, error)
	// CatFileExists reports whether rev resolves to an existing object.
	CatFileExists(rev string) bool
	// Show returns the contents of file at commit (git show commit:file).
	Show(commit, file string) ([]byte, error)
	// DiffNameOnly lists files changed between base and head.
	DiffNameOnly(base, head string) ([]string, error)
	// SubmoduleAdd registers url as a submodule at path.
	SubmoduleAdd(url, path string) error
	// SubmoduleUpdateInit initialises submodules, optionally recursively.
	SubmoduleUpdateInit(recursive bool) error
	// SubmoduleRemove fully removes the submodule at path (rollback helper).
	SubmoduleRemove(path string) error
}

// ExecGit is the exec-backed implementation of Git.
type ExecGit struct {
	exec executil.Executor
}

// New returns an ExecGit backed by the OS executor.
func New() *ExecGit {
	return &ExecGit{exec: executil.NewOSExecutor()}
}

// NewWithExecutor returns an ExecGit backed by the supplied executor (tests).
func NewWithExecutor(e executil.Executor) *ExecGit {
	return &ExecGit{exec: e}
}

// run executes git with args, returning stdout. stderr is included in errors.
func (g *ExecGit) run(args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("git: no arguments provided")
	}
	cmd := g.exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("git %s: %w: %s", args[0], err, msg)
		}
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return stdout.Bytes(), nil
}

// Toplevel returns the absolute path of the working tree root.
func (g *ExecGit) Toplevel() (string, error) {
	out, err := g.run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CatFileExists reports whether rev resolves to an existing object.
func (g *ExecGit) CatFileExists(rev string) bool {
	_, err := g.run("cat-file", "-e", rev)
	return err == nil
}

// Show returns the contents of file at commit.
func (g *ExecGit) Show(commit, file string) ([]byte, error) {
	return g.run("show", fmt.Sprintf("%s:%s", commit, file))
}

// DiffNameOnly lists files changed between base and head.
func (g *ExecGit) DiffNameOnly(base, head string) ([]string, error) {
	out, err := g.run("diff", "--name-only", base, head)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			files = append(files, s)
		}
	}
	return files, nil
}

// SubmoduleAdd registers url as a submodule at path.
func (g *ExecGit) SubmoduleAdd(url, path string) error {
	_, err := g.run("submodule", "add", url, path)
	return err
}

// SubmoduleUpdateInit initialises submodules, optionally recursively.
func (g *ExecGit) SubmoduleUpdateInit(recursive bool) error {
	args := []string{"submodule", "update", "--init"}
	if recursive {
		args = append(args, "--recursive")
	}
	_, err := g.run(args...)
	return err
}

// SubmoduleRemove fully removes the submodule at path. It runs every cleanup
// step best-effort (continuing past failures) and returns the joined errors so
// callers can diagnose partial cleanup:
//  1. git submodule deinit -f <path>
//  2. git rm -f <path>             (removes the working tree + .gitmodules entry)
//  3. delete <git-common-dir>/modules/<path> from the filesystem
//
// Step 3 is a filesystem removal (not a git command) and is anchored at the
// real git directory via rev-parse --git-common-dir, so it works inside linked
// worktrees where the module store does not live under <worktree>/.git.
func (g *ExecGit) SubmoduleRemove(path string) error {
	var errs []error
	if _, err := g.run("submodule", "deinit", "-f", path); err != nil {
		errs = append(errs, err)
	}
	if _, err := g.run("rm", "-f", path); err != nil {
		errs = append(errs, err)
	}
	if moduleDir, err := g.submoduleGitDir(path); err != nil {
		errs = append(errs, err)
	} else if err := os.RemoveAll(moduleDir); err != nil {
		errs = append(errs, fmt.Errorf("remove module dir %q: %w", moduleDir, err))
	}
	return errors.Join(errs...)
}

// submoduleGitDir resolves the filesystem location of a submodule's git data
// within the module store (<git-common-dir>/modules/<path>).
func (g *ExecGit) submoduleGitDir(path string) (string, error) {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) || strings.HasPrefix(path, "..") || path == ".." {
		return "", fmt.Errorf("gitutil: submodule path %q must be a clean relative path and cannot contain directory traversal segments", path)
	}
	out, err := g.run("rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	gitDir := strings.TrimSpace(string(out))
	if gitDir == "" {
		gitDir = ".git"
	}
	return filepath.Join(gitDir, "modules", path), nil
}

// Ensure ExecGit satisfies Git.
var _ Git = (*ExecGit)(nil)
