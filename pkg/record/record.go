package record

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

var (
	appIDRegexp  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)
	archRegexp   = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	branchRegexp = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// Record is an immutable snapshot of one published (app, arch, branch) cell.
type Record struct {
	AppID    string `json:"app-id"`
	Arch     string `json:"arch"`
	Branch   string `json:"branch"`
	Name     string `json:"name"`
	Registry string `json:"registry"`
	Digest   string `json:"digest"`
	Ref      string `json:"ref"`
	Tag      string `json:"tag"`
}

// RecordWithLabels holds a parsed Record and its associated OCI labels.
type RecordWithLabels struct {
	Record Record
	Labels map[string]string
	Path   string
}

// Validate asserts that the Record has the required fields and is safe for path generation.
func (r Record) Validate() error {
	if r.AppID == "" || r.Arch == "" {
		return fmt.Errorf("Record requires app-id and arch")
	}

	if !appIDRegexp.MatchString(r.AppID) {
		return fmt.Errorf("Record app-id %q is invalid (must match %s)", r.AppID, appIDRegexp.String())
	}

	if !archRegexp.MatchString(r.Arch) {
		return fmt.Errorf("Record arch %q is invalid (must match %s)", r.Arch, archRegexp.String())
	}

	// Records written before the branch became part of the cell path carry no
	// branch, so only validate it when set.
	if r.Branch != "" && !branchRegexp.MatchString(r.Branch) {
		return fmt.Errorf("Record branch %q is invalid (must match %s)", r.Branch, branchRegexp.String())
	}

	return nil
}

// CellDir resolves the cell directory path under root. The branch is part of
// the path so that publishing several branches of one app and arch in a single
// execution does not have each push overwrite the last one's record.
func (r Record) CellDir(root string) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	if r.Branch == "" {
		return filepath.Join(root, legacyCellName(r)), nil
	}
	return filepath.Join(root, fmt.Sprintf("%s-%s-%s", r.AppID, r.Branch, r.Arch)), nil
}

// legacyCellName is the pre-branch cell directory name (<app>-<arch>).
func legacyCellName(r Record) string {
	return fmt.Sprintf("%s-%s", r.AppID, r.Arch)
}

// WriteRecord writes the record and labels into a cell directory under root.
func WriteRecord(root string, r Record, labels map[string]string) (string, error) {
	cellDir, err := r.CellDir(root)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(cellDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cell directory: %w", err)
	}

	// Write record.json
	recPath := filepath.Join(cellDir, "record.json")
	recBytes, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal record: %w", err)
	}
	// Append newline
	recBytes = append(recBytes, '\n')
	if err := os.WriteFile(recPath, recBytes, 0644); err != nil {
		return "", fmt.Errorf("failed to write record.json: %w", err)
	}

	// Write labels.json
	lblPath := filepath.Join(cellDir, "labels.json")
	if labels == nil {
		labels = make(map[string]string)
	}
	lblBytes, err := json.MarshalIndent(labels, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal labels: %w", err)
	}
	// Append newline
	lblBytes = append(lblBytes, '\n')
	if err := os.WriteFile(lblPath, lblBytes, 0644); err != nil {
		return "", fmt.Errorf("failed to write labels.json: %w", err)
	}

	// Touch .aetherpak-cells in root
	cellsMarker := filepath.Join(root, ".aetherpak-cells")
	if err := os.WriteFile(cellsMarker, []byte{}, 0644); err != nil {
		return "", fmt.Errorf("failed to touch .aetherpak-cells: %w", err)
	}

	return cellDir, nil
}

// IterRecords yields all complete records and labels found under root.
func IterRecords(root string) ([]RecordWithLabels, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Return empty list, not error, when root is absent
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	var cellDirs []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			recPath := filepath.Join(path, "record.json")
			lblPath := filepath.Join(path, "labels.json")
			if fileExists(recPath) && fileExists(lblPath) {
				cellDirs = append(cellDirs, path)
				return filepath.SkipDir // Optimize: skip walking inside the matched cell directory
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to traverse records directory: %w", err)
	}

	// We want to sort directory names to keep traversal deterministic
	sort.Strings(cellDirs)

	var results []RecordWithLabels
	// seen maps a logical cell (app, arch, branch) to its index in results, so a
	// legacy cell cannot shadow the branch-qualified cell that replaces it on
	// disk. Both forms carry the same branch in JSON, so the directory shape is
	// the only signal that tells them apart.
	seen := make(map[cellKey]int, len(cellDirs))
	for _, cellPath := range cellDirs {
		recPath := filepath.Join(cellPath, "record.json")
		lblPath := filepath.Join(cellPath, "labels.json")

		recBytes, err := os.ReadFile(recPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read record file %q: %w", recPath, err)
		}
		var r Record
		if err := json.Unmarshal(recBytes, &r); err != nil {
			return nil, fmt.Errorf("failed to parse record JSON from %q: %w", recPath, err)
		}

		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("invalid record at %q: %w", recPath, err)
		}

		lblBytes, err := os.ReadFile(lblPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read labels file %q: %w", lblPath, err)
		}
		var labels map[string]string
		if err := json.Unmarshal(lblBytes, &labels); err != nil {
			return nil, fmt.Errorf("failed to parse labels JSON from %q: %w", lblPath, err)
		}

		rwl := RecordWithLabels{
			Record: r,
			Labels: labels,
			Path:   cellPath,
		}

		key := cellKey{AppID: r.AppID, Arch: r.Arch, Branch: r.Branch}
		if idx, ok := seen[key]; ok {
			// Prefer the branch-qualified cell over the legacy <app>-<arch>
			// form. On equal preference the first cell wins, which keeps the
			// result deterministic.
			if isLegacyCell(results[idx].Path, results[idx].Record) && !isLegacyCell(rwl.Path, rwl.Record) {
				results[idx] = rwl
			}
			continue
		}
		seen[key] = len(results)
		results = append(results, rwl)
	}

	return results, nil
}

// cellKey identifies the logical cell a record belongs to, independent of the
// directory shape the writing version used.
type cellKey struct {
	AppID  string
	Arch   string
	Branch string
}

// isLegacyCell reports whether a cell was written to the pre-branch path form
// (<app>-<arch>). Records written before the branch became part of the cell path
// still carry a branch in JSON, so the directory name is the only reliable
// signal.
func isLegacyCell(cellPath string, r Record) bool {
	if r.Branch == "" {
		return false
	}
	return filepath.Base(cellPath) == legacyCellName(r)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
