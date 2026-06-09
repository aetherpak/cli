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

// validRefTypes lists the OSTree ref type prefixes that are recognized.
var validRefTypes = []string{"app", "runtime"}

// Info holds the coordinates resolved from a repo's first app/* or runtime/* ref.
type Info struct {
	AppID, Arch, Branch, RepoPath, RefType string
}

// Ref returns the full OSTree ref string: <RefType>/<AppID>/<Arch>/<Branch>.
func (i Info) Ref() string {
	return fmt.Sprintf("%s/%s/%s/%s", i.RefType, i.AppID, i.Arch, i.Branch)
}

func isValidRefType(t string) bool {
	for _, v := range validRefTypes {
		if t == v {
			return true
		}
	}
	return false
}

func parseRef(ref string) (refType, id, arch, branch string, err error) {
	parts := strings.Split(strings.TrimSpace(ref), "/")
	if len(parts) != 4 || !isValidRefType(parts[0]) {
		return "", "", "", "", fmt.Errorf("repoinfo: not a valid ref: %q", ref)
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

// Resolve returns the coordinates of the first app/* or runtime/* ref in the repo.
// It first attempts a pure Go directory traversal over <repoPath>/refs/heads to find
// the ref, and falls back to invoking the "ostree" host binary if needed.
func Resolve(repoPath string) (Info, error) {
	_ = RestoreEmptyDirs(repoPath)
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
		for _, prefix := range validRefTypes {
			if strings.HasPrefix(ref, prefix+"/") {
				foundRef = ref
				return fmt.Errorf("stop walk") // sentinel error to abort walking
			}
		}
		return nil
	})

	if foundRef != "" {
		refType, id, arch, branch, err := parseRef(foundRef)
		if err == nil {
			return Info{AppID: id, Arch: arch, Branch: branch, RepoPath: repoPath, RefType: refType}, nil
		}
	}

	// Fallback: execute ostree host binary
	out, err := execCommand("ostree", "refs", "--repo="+repoPath).Output()
	if err != nil {
		return Info{}, fmt.Errorf("repoinfo: ostree refs: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		for _, prefix := range validRefTypes {
			if strings.HasPrefix(trimmed, prefix+"/") {
				refType, id, arch, branch, err := parseRef(trimmed)
				if err != nil {
					return Info{}, err
				}
				return Info{AppID: id, Arch: arch, Branch: branch, RepoPath: repoPath, RefType: refType}, nil
			}
		}
	}
	return Info{}, fmt.Errorf("repoinfo: no app/* or runtime/* ref in %s", repoPath)
}

// ResolveAll returns the coordinates of all app/* and runtime/* refs in the repo.
// It first attempts a pure Go directory traversal over <repoPath>/refs/heads to find
// the refs, and falls back to invoking the "ostree" host binary if needed.
func ResolveAll(repoPath string) ([]Info, error) {
	_ = RestoreEmptyDirs(repoPath)
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
		for _, prefix := range validRefTypes {
			if strings.HasPrefix(ref, prefix+"/") {
				foundRefs = append(foundRefs, ref)
				break
			}
		}
		return nil
	})

	var results []Info
	for _, foundRef := range foundRefs {
		refType, id, arch, branch, err := parseRef(foundRef)
		if err == nil {
			results = append(results, Info{AppID: id, Arch: arch, Branch: branch, RepoPath: repoPath, RefType: refType})
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
		for _, prefix := range validRefTypes {
			if strings.HasPrefix(trimmed, prefix+"/") {
				refType, id, arch, branch, err := parseRef(trimmed)
				if err == nil {
					results = append(results, Info{AppID: id, Arch: arch, Branch: branch, RepoPath: repoPath, RefType: refType})
				}
				break
			}
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("repoinfo: no app/* or runtime/* refs in %s", repoPath)
	}
	return results, nil
}
