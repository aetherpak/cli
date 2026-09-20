package record

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordValidation(t *testing.T) {
	tests := []struct {
		name    string
		rec     Record
		wantErr bool
	}{
		{
			name: "valid record",
			rec: Record{
				AppID: "org.example.App",
				Arch:  "x86_64",
			},
			wantErr: false,
		},
		{
			name: "missing app-id",
			rec: Record{
				Arch: "x86_64",
			},
			wantErr: true,
		},
		{
			name: "missing arch",
			rec: Record{
				AppID: "org.example.App",
			},
			wantErr: true,
		},
		{
			name: "path traversal in app-id",
			rec: Record{
				AppID: "../escaped",
				Arch:  "x86_64",
			},
			wantErr: true,
		},
		{
			name: "null byte in app-id",
			rec: Record{
				AppID: "org.example\x00App",
				Arch:  "x86_64",
			},
			wantErr: true,
		},
		{
			name: "spaces in app-id",
			rec: Record{
				AppID: "org.example App",
				Arch:  "x86_64",
			},
			wantErr: true,
		},
		{
			name: "valid branch",
			rec: Record{
				AppID:  "org.example.App",
				Arch:   "x86_64",
				Branch: "2.54",
			},
			wantErr: false,
		},
		{
			name: "path traversal in branch",
			rec: Record{
				AppID:  "org.example.App",
				Arch:   "x86_64",
				Branch: "../escaped",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Record.Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestWriteAndIterRecords(t *testing.T) {
	tempDir := t.TempDir()

	rec1 := Record{
		AppID:    "org.example.AppA",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "my-org/my-app-a",
		Registry: "ghcr.io",
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		Ref:      "app/org.example.AppA/x86_64/stable",
		Tag:      "org_example_AppA-stable-x86_64",
	}

	labels1 := map[string]string{
		"org.flatpak.ref":    "app/org.example.AppA/x86_64/stable",
		"org.flatpak.commit": "abcdef123456",
	}

	cellDir, err := WriteRecord(tempDir, rec1, labels1)
	if err != nil {
		t.Fatalf("failed to write record 1: %v", err)
	}

	// Verify cell directory exists and has records
	if _, err := os.Stat(filepath.Join(cellDir, "record.json")); err != nil {
		t.Errorf("record.json not found in cell: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cellDir, "labels.json")); err != nil {
		t.Errorf("labels.json not found in cell: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".aetherpak-cells")); err != nil {
		t.Errorf(".aetherpak-cells marker not found: %v", err)
	}

	// Write a second record
	rec2 := Record{
		AppID:    "org.example.AppB",
		Arch:     "aarch64",
		Branch:   "beta",
		Name:     "my-org/my-app-b",
		Registry: "ghcr.io",
		Digest:   "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		Ref:      "app/org.example.AppB/aarch64/beta",
		Tag:      "org_example_AppB-beta-aarch64",
	}

	labels2 := map[string]string{
		"org.flatpak.ref":    "app/org.example.AppB/aarch64/beta",
		"org.flatpak.commit": "7890abcdef12",
	}

	_, err = WriteRecord(tempDir, rec2, labels2)
	if err != nil {
		t.Fatalf("failed to write record 2: %v", err)
	}

	// Iter records
	records, err := IterRecords(tempDir)
	if err != nil {
		t.Fatalf("failed to iter records: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	// Iterate should return sorted by cell directory name:
	// AetherPak directory name for rec1: org.example.AppA-stable-x86_64
	// AetherPak directory name for rec2: org.example.AppB-beta-aarch64
	// Sorted:
	// 1st: org.example.AppA-stable-x86_64 (rec1)
	// 2nd: org.example.AppB-beta-aarch64 (rec2)
	if records[0].Record.AppID != "org.example.AppA" {
		t.Errorf("expected first sorted record to be org.example.AppA, got %s", records[0].Record.AppID)
	}
	if records[1].Record.AppID != "org.example.AppB" {
		t.Errorf("expected second sorted record to be org.example.AppB, got %s", records[1].Record.AppID)
	}

	if records[0].Labels["org.flatpak.commit"] != "abcdef123456" {
		t.Errorf("incorrect label value: got %s", records[0].Labels["org.flatpak.commit"])
	}
}

func TestIterRecordsFailsOnInvalid(t *testing.T) {
	tempDir := t.TempDir()

	// Write a valid record first
	recValid := Record{
		AppID: "org.example.AppValid",
		Arch:  "x86_64",
	}
	_, err := WriteRecord(tempDir, recValid, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Write an invalid record manually to bypass WriteRecord validation
	invalidCellDir := filepath.Join(tempDir, "org.example.AppInvalid-x86_64")
	if err := os.MkdirAll(invalidCellDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Missing arch and has slash in AppID
	invalidRecJSON := `{"app-id": "org.example/AppInvalid", "arch": ""}`
	if err := os.WriteFile(filepath.Join(invalidCellDir, "record.json"), []byte(invalidRecJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidCellDir, "labels.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = IterRecords(tempDir)
	if err == nil {
		t.Fatal("expected IterRecords to fail on invalid/corrupt record cells, but it succeeded")
	}
}

func TestIterRecordsRecursive(t *testing.T) {
	tempDir := t.TempDir()

	cellADir := filepath.Join(tempDir, "subdirA", "org.example.AppA-x86_64")
	cellBDir := filepath.Join(tempDir, "subdirB", "nested", "org.example.AppB-aarch64")

	if err := os.MkdirAll(cellADir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cellBDir, 0755); err != nil {
		t.Fatal(err)
	}

	recA := `{"app-id": "org.example.AppA", "arch": "x86_64"}`
	recB := `{"app-id": "org.example.AppB", "arch": "aarch64"}`

	if err := os.WriteFile(filepath.Join(cellADir, "record.json"), []byte(recA), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cellADir, "labels.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cellBDir, "record.json"), []byte(recB), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cellBDir, "labels.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	records, err := IterRecords(tempDir)
	if err != nil {
		t.Fatalf("failed to iter records: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	if records[0].Record.AppID != "org.example.AppA" {
		t.Errorf("expected first record to be AppA, got %s", records[0].Record.AppID)
	}
	if records[0].Path != cellADir {
		t.Errorf("expected path to be %s, got %s", cellADir, records[0].Path)
	}

	if records[1].Record.AppID != "org.example.AppB" {
		t.Errorf("expected second record to be AppB, got %s", records[1].Record.AppID)
	}
	if records[1].Path != cellBDir {
		t.Errorf("expected path to be %s, got %s", cellBDir, records[1].Path)
	}
}

// writeRawCell places a record and labels directly in cellDir, bypassing
// WriteRecord, so a test can reproduce the legacy <app>-<arch> layout a
// pre-branch version of this package produced.
func writeRawCell(t *testing.T, cellDir string, rec Record, labels map[string]string) {
	t.Helper()
	if err := os.MkdirAll(cellDir, 0755); err != nil {
		t.Fatal(err)
	}
	recBytes, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cellDir, "record.json"), recBytes, 0644); err != nil {
		t.Fatal(err)
	}
	lblBytes, err := json.Marshal(labels)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cellDir, "labels.json"), lblBytes, 0644); err != nil {
		t.Fatal(err)
	}
}

const (
	freshDigest  = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	legacyDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

// TestIterRecordsPrefersBranchQualifiedCell covers both lexical orders between
// the legacy <app>-<arch> cell and the branch-qualified <app>-<branch>-<arch>
// cell: only the architecture decides whether the legacy cell is seen before or
// after its replacement, so both paths through the dedupe are exercised.
func TestIterRecordsPrefersBranchQualifiedCell(t *testing.T) {
	const appID = "org.example.App"
	const branch = "stable"

	for _, arch := range []string{"x86_64", "aarch64"} {
		t.Run(arch, func(t *testing.T) {
			tempDir := t.TempDir()
			ref := "app/" + appID + "/" + arch + "/" + branch

			// Fresh cell written by the current code: <app>-<branch>-<arch>.
			fresh := Record{
				AppID:    appID,
				Arch:     arch,
				Branch:   branch,
				Name:     "my-org/my-app",
				Registry: "ghcr.io",
				Digest:   freshDigest,
				Ref:      ref,
				Tag:      "fresh",
			}
			freshLabels := map[string]string{
				"org.flatpak.ref":    ref,
				"org.flatpak.commit": "fresh",
			}
			freshCell, err := WriteRecord(tempDir, fresh, freshLabels)
			if err != nil {
				t.Fatalf("failed to write fresh record: %v", err)
			}
			if filepath.Base(freshCell) != appID+"-"+branch+"-"+arch {
				t.Fatalf("fresh cell = %q, want branch-qualified directory", freshCell)
			}

			// Legacy cell for the same app/arch/branch, written to the pre-branch
			// path.
			legacy := fresh
			legacy.Digest = legacyDigest
			legacy.Tag = "legacy"
			writeRawCell(t, filepath.Join(tempDir, appID+"-"+arch), legacy, map[string]string{
				"org.flatpak.ref":    ref,
				"org.flatpak.commit": "legacy",
			})

			records, err := IterRecords(tempDir)
			if err != nil {
				t.Fatalf("failed to iter records: %v", err)
			}
			if len(records) != 1 {
				t.Fatalf("expected the legacy cell to be dropped, got %d records: %+v", len(records), records)
			}
			if records[0].Path != freshCell {
				t.Errorf("surviving cell path = %q, want %q", records[0].Path, freshCell)
			}
			if records[0].Record.Digest != freshDigest {
				t.Errorf("surviving digest = %q, want %q", records[0].Record.Digest, freshDigest)
			}
			if records[0].Labels["org.flatpak.commit"] != "fresh" {
				t.Errorf("surviving labels = %v, want the branch-qualified cell's", records[0].Labels)
			}
		})
	}
}

func TestIterRecordsKeepsLegacyCellWithoutCounterpart(t *testing.T) {
	tempDir := t.TempDir()

	const appID = "org.example.App"
	const arch = "x86_64"
	const branch = "stable"
	ref := "app/" + appID + "/" + arch + "/" + branch

	// A legacy cell with no branch-qualified sibling is still the only record for
	// its app/arch/branch, so it must keep loading (backward compatibility).
	legacyCell := filepath.Join(tempDir, appID+"-"+arch)
	writeRawCell(t, legacyCell, Record{
		AppID:    appID,
		Arch:     arch,
		Branch:   branch,
		Name:     "my-org/my-app",
		Registry: "ghcr.io",
		Digest:   legacyDigest,
		Ref:      ref,
		Tag:      "legacy",
	}, map[string]string{
		"org.flatpak.ref":    ref,
		"org.flatpak.commit": "legacy",
	})

	records, err := IterRecords(tempDir)
	if err != nil {
		t.Fatalf("failed to iter records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Path != legacyCell {
		t.Errorf("surviving cell path = %q, want %q", records[0].Path, legacyCell)
	}
	if records[0].Record.Digest != legacyDigest {
		t.Errorf("surviving digest = %q, want %q", records[0].Record.Digest, legacyDigest)
	}
}

func TestWriteRecordSeparatesBranches(t *testing.T) {
	tempDir := t.TempDir()

	rec := Record{
		AppID:    "org.example.App",
		Arch:     "x86_64",
		Name:     "my-org/my-app",
		Registry: "ghcr.io",
	}

	stable := rec
	stable.Branch = "master"
	stable.Digest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	stable.Ref = "app/org.example.App/x86_64/master"

	release := rec
	release.Branch = "2.54"
	release.Digest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	release.Ref = "app/org.example.App/x86_64/2.54"

	stableCell, err := WriteRecord(tempDir, stable, map[string]string{"org.flatpak.ref": stable.Ref})
	if err != nil {
		t.Fatalf("failed to write master record: %v", err)
	}
	releaseCell, err := WriteRecord(tempDir, release, map[string]string{"org.flatpak.ref": release.Ref})
	if err != nil {
		t.Fatalf("failed to write 2.54 record: %v", err)
	}

	if stableCell == releaseCell {
		t.Fatalf("expected distinct cells for two branches of one app and arch, got %q for both", stableCell)
	}

	records, err := IterRecords(tempDir)
	if err != nil {
		t.Fatalf("failed to iter records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	digests := map[string]string{}
	for _, rwl := range records {
		digests[rwl.Record.Branch] = rwl.Record.Digest
	}
	if digests["master"] != stable.Digest {
		t.Errorf("master record digest = %q, want %q", digests["master"], stable.Digest)
	}
	if digests["2.54"] != release.Digest {
		t.Errorf("2.54 record digest = %q, want %q", digests["2.54"], release.Digest)
	}
}
