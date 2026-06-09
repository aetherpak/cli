package repoinfo

import (
	"os"
	"path/filepath"
)

// RestoreEmptyDirs checks if the given repoPath exists and is a directory.
// If it contains an OSTree repo (or is intended to be one), it ensures
// the required empty directories (refs/heads, refs/mirrors, refs/remotes) exist,
// which is useful since CI runner artifact upload/download systems strip empty directories.
func RestoreEmptyDirs(repoPath string) error {
	fi, err := os.Stat(repoPath)
	if err != nil {
		// If the directory does not exist, let it be resolved or fail later naturally.
		return nil
	}
	if !fi.IsDir() {
		return nil
	}

	// Recreate directories that are required by ostree/flatpak but might be dropped
	// during artifact upload/download.
	subdirs := []string{
		filepath.Join(repoPath, "refs", "heads"),
		filepath.Join(repoPath, "refs", "mirrors"),
		filepath.Join(repoPath, "refs", "remotes"),
	}

	for _, dir := range subdirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}
