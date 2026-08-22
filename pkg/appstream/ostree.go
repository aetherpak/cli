package appstream

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/executil"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"sync"
)

var ostreeRepoMutex sync.Mutex

// FindMetainfoPathInCommit searches the OSTree ref tree for an AppStream metainfo or appdata XML file.
func FindMetainfoPathInCommit(executor executil.Executor, repoPath, ref string) (string, error) {
	if executor == nil {
		executor = executil.NewOSExecutor()
	}

	// Potential search subdirectories in priority order
	searchPaths := []string{
		"/files/share/metainfo",
		"/files/share/appdata",
	}

	for _, subpath := range searchPaths {
		cmd := executor.Command("ostree", "ls", "--repo="+repoPath, ref, subpath)
		var stdout bytes.Buffer
		cmd.SetStdout(&stdout)
		if err := cmd.Run(); err == nil {
			lines := strings.Split(stdout.String(), "\n")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				fields := strings.Fields(trimmed)
				if len(fields) > 0 {
					filename := fields[len(fields)-1]
					if strings.HasSuffix(filename, ".metainfo.xml") || strings.HasSuffix(filename, ".appdata.xml") || strings.HasSuffix(filename, ".xml") {
						if strings.HasPrefix(filename, "/") {
							return filename, nil
						}
						return filepath.ToSlash(filepath.Join(subpath, filename)), nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("appstream: no metainfo or appdata XML file found in ref %s", ref)
}

// ReadMetainfoFromCommit extracts the file content at subpath from the given OSTree commit.
func ReadMetainfoFromCommit(executor executil.Executor, repoPath, ref, subpath string) ([]byte, error) {
	if executor == nil {
		executor = executil.NewOSExecutor()
	}

	cmd := executor.Command("ostree", "cat", "--repo="+repoPath, ref, subpath)
	var stdout, stderr bytes.Buffer
	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("appstream: failed to read %s from ref %s (%w): %s", subpath, ref, err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// SyncOSTreeCommitAppStream updates the AppStream metainfo XML file inside an OSTree ref,
// re-commits the modified tree, and triggers flatpak build-update-repo to refresh the repository AppStream catalog.
func SyncOSTreeCommitAppStream(executor executil.Executor, repoPath, ref string, opts ReleaseOptions) (bool, error) {
	if executor == nil {
		executor = executil.NewOSExecutor()
	}

	ostreeRepoMutex.Lock()
	defer ostreeRepoMutex.Unlock()

	metainfoPath, err := FindMetainfoPathInCommit(executor, repoPath, ref)
	if err != nil {
		logger.Debug("Skipping AppStream release sync for %s: %v", ref, err)
		return false, nil
	}

	xmlData, err := ReadMetainfoFromCommit(executor, repoPath, ref, metainfoPath)
	if err != nil {
		return false, err
	}

	updatedXML, modified, err := SyncRelease(xmlData, opts)
	if err != nil {
		return false, fmt.Errorf("appstream: failed to synchronize release XML for %s: %w", ref, err)
	}

	if !modified {
		logger.Debug("AppStream release metadata for %s is already up-to-date (version: %s)", ref, opts.Version)
		// Ensure catalog refresh is retryable even if commit already contains release
		updateCatalogCmd := executor.Command("flatpak", "build-update-repo",
			"--no-update-summary",
			repoPath,
		)
		var updateStderr bytes.Buffer
		updateCatalogCmd.SetStderr(&updateStderr)
		if err := updateCatalogCmd.Run(); err != nil {
			return false, fmt.Errorf("appstream: failed to update repository catalog (%w): %s", err, updateStderr.String())
		}
		return false, nil
	}

	// Checkout commit to a temporary working directory
	tempDir, err := os.MkdirTemp("", "aetherpak-ostree-checkout-*")
	if err != nil {
		return false, fmt.Errorf("appstream: failed to create temporary checkout directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	checkoutCmd := executor.Command("ostree", "checkout", "--repo="+repoPath, "--user-mode", ref, tempDir)
	var checkoutStderr bytes.Buffer
	checkoutCmd.SetStderr(&checkoutStderr)
	if err := checkoutCmd.Run(); err != nil {
		return false, fmt.Errorf("appstream: ostree checkout failed for %s (%w): %s", ref, err, checkoutStderr.String())
	}

	// Write modified metainfo XML file
	relFile := strings.TrimPrefix(filepath.Clean("/"+metainfoPath), "/")
	targetPath := filepath.Join(tempDir, relFile)
	realTempDir, err := filepath.EvalSymlinks(tempDir)
	if err != nil {
		realTempDir = tempDir
	}

	// Verify parent directory does not escape tempDir
	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return false, fmt.Errorf("appstream: failed to create parent directories for %s: %w", targetPath, err)
	}
	realParentDir, err := filepath.EvalSymlinks(parentDir)
	if err != nil {
		return false, fmt.Errorf("appstream: failed to resolve parent directory for %s: %w", targetPath, err)
	}
	relParent, err := filepath.Rel(realTempDir, realParentDir)
	if err != nil || strings.HasPrefix(relParent, "..") {
		return false, fmt.Errorf("appstream: metainfo parent path %s escapes checkout directory", parentDir)
	}

	// Verify target file is not a symlink and is a regular file if it exists
	if fi, err := os.Lstat(targetPath); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			return false, fmt.Errorf("appstream: metainfo target %s is a symlink or non-regular file", targetPath)
		}
	}

	if err := os.WriteFile(targetPath, updatedXML, 0644); err != nil {
		return false, fmt.Errorf("appstream: failed to write updated metainfo file: %w", err)
	}

	// Discover and preserve metadata keys from original commit (e.g. xa.metadata, xa.ref, ostree.ref-binding)
	listKeysCmd := executor.Command("ostree", "show", "--repo="+repoPath, "--list-metadata-keys", ref)
	var keysStdout bytes.Buffer
	listKeysCmd.SetStdout(&keysStdout)
	var metadataArgs []string
	metadataArgs = append(metadataArgs, "--parent="+ref)
	if err := listKeysCmd.Run(); err == nil && keysStdout.Len() > 0 {
		lines := strings.Split(keysStdout.String(), "\n")
		for _, k := range lines {
			k = strings.TrimSpace(k)
			if k != "" {
				metadataArgs = append(metadataArgs, "--keep-metadata="+k)
			}
		}
	} else {
		// Fallback to standard Flatpak metadata keys if listing fails
		metadataArgs = append(metadataArgs,
			"--keep-metadata=xa.metadata",
			"--keep-metadata=xa.ref",
			"--keep-metadata=xa.cache",
			"--keep-metadata=ostree.ref-binding",
		)
	}

	// Commit updated tree back to OSTree ref
	commitSubject := fmt.Sprintf("Dynamic AppStream release sync: %s (%s)", opts.Version, opts.Date)
	commitArgs := []string{
		"commit",
		"--repo=" + repoPath,
		"--branch=" + ref,
		"--tree=dir=" + tempDir,
		"--subject=" + commitSubject,
	}
	commitArgs = append(commitArgs, metadataArgs...)
	commitCmd := executor.Command("ostree", commitArgs...)
	var commitStderr bytes.Buffer
	commitCmd.SetStderr(&commitStderr)
	if err := commitCmd.Run(); err != nil {
		return false, fmt.Errorf("appstream: ostree commit failed for %s (%w): %s", ref, err, commitStderr.String())
	}

	// Refresh repository AppStream catalog branch (appstream/<arch>)
	updateCatalogCmd := executor.Command("flatpak", "build-update-repo",
		"--no-update-summary",
		repoPath,
	)
	var updateStderr bytes.Buffer
	updateCatalogCmd.SetStderr(&updateStderr)
	if err := updateCatalogCmd.Run(); err != nil {
		return true, fmt.Errorf("appstream: failed to update repository catalog (%w): %s", err, updateStderr.String())
	}

	logger.Info("Synchronized AppStream release metadata for %s to version %s (%s)", ref, opts.Version, opts.Date)
	return true, nil
}
