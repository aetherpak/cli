package repoinfo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// For testing
var execCommand = exec.Command

// Info holds the coordinates resolved from a repo's first app/* ref.
type Info struct {
	AppID, Arch, Branch, RepoPath string
}

func parseRef(ref string) (id, arch, branch string, err error) {
	parts := strings.Split(strings.TrimSpace(ref), "/")
	if len(parts) != 4 || parts[0] != "app" {
		return "", "", "", fmt.Errorf("repoinfo: not an app ref: %q", ref)
	}
	return parts[1], parts[2], parts[3], nil
}

// Resolve returns the coordinates of the first app/* ref in the repo.
// It first attempts a pure Go directory traversal over <repoPath>/refs/heads to find
// the ref, and falls back to invoking the "ostree" host binary if needed.
func Resolve(repoPath string) (Info, error) {
	headsDir := filepath.Join(repoPath, "refs", "heads")
	var foundRef string
	_ = filepath.Walk(headsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(headsDir, path)
		if err != nil {
			return err
		}
		ref := filepath.ToSlash(rel)
		if strings.HasPrefix(ref, "app/") {
			foundRef = ref
			return fmt.Errorf("stop walk") // sentinel error to abort walking
		}
		return nil
	})

	if foundRef != "" {
		id, arch, branch, err := parseRef(foundRef)
		if err == nil {
			return Info{AppID: id, Arch: arch, Branch: branch, RepoPath: repoPath}, nil
		}
	}

	// Fallback: execute ostree host binary
	out, err := execCommand("ostree", "refs", "--repo="+repoPath).Output()
	if err != nil {
		return Info{}, fmt.Errorf("repoinfo: ostree refs: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "app/") {
			id, arch, branch, err := parseRef(line)
			if err != nil {
				return Info{}, err
			}
			return Info{AppID: id, Arch: arch, Branch: branch, RepoPath: repoPath}, nil
		}
	}
	return Info{}, fmt.Errorf("repoinfo: no app/* ref in %s", repoPath)
}

// ResolveAll returns the coordinates of all app/* refs in the repo.
// It first attempts a pure Go directory traversal over <repoPath>/refs/heads to find
// the refs, and falls back to invoking the "ostree" host binary if needed.
func ResolveAll(repoPath string) ([]Info, error) {
	headsDir := filepath.Join(repoPath, "refs", "heads")
	var foundRefs []string
	_ = filepath.Walk(headsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(headsDir, path)
		if err != nil {
			return err
		}
		ref := filepath.ToSlash(rel)
		if strings.HasPrefix(ref, "app/") {
			foundRefs = append(foundRefs, ref)
		}
		return nil
	})

	var results []Info
	for _, foundRef := range foundRefs {
		id, arch, branch, err := parseRef(foundRef)
		if err == nil {
			results = append(results, Info{AppID: id, Arch: arch, Branch: branch, RepoPath: repoPath})
		}
	}

	if len(results) > 0 {
		return results, nil
	}

	// Fallback: execute ostree host binary
	out, err := execCommand("ostree", "refs", "--repo="+repoPath).Output()
	if err != nil {
		return nil, fmt.Errorf("repoinfo: ostree refs: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "app/") {
			id, arch, branch, err := parseRef(trimmed)
			if err == nil {
				results = append(results, Info{AppID: id, Arch: arch, Branch: branch, RepoPath: repoPath})
			}
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("repoinfo: no app/* refs in %s", repoPath)
	}
	return results, nil
}
