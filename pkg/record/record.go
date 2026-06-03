package record

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Record is an immutable snapshot of one published (app, arch) cell.
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

	for _, field := range []struct {
		name  string
		value string
	}{
		{"app-id", r.AppID},
		{"arch", r.Arch},
	} {
		if strings.Contains(field.value, "/") || strings.Contains(field.value, "\\") || field.value == "." || field.value == ".." {
			return fmt.Errorf("Record %q must not contain path separators or traversal segments", field.name)
		}
	}

	return nil
}

// CellDir resolves the cell directory path under root.
func (r Record) CellDir(root string) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return filepath.Join(root, fmt.Sprintf("%s-%s", r.AppID, r.Arch)), nil
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

		results = append(results, RecordWithLabels{
			Record: r,
			Labels: labels,
			Path:   cellPath,
		})
	}

	return results, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
