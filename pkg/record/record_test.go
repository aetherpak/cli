package record

import (
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
	// AetherPak directory name for rec1: org.example.AppA-x86_64
	// AetherPak directory name for rec2: org.example.AppB-aarch64
	// Sorted:
	// 1st: org.example.AppA-x86_64 (rec1)
	// 2nd: org.example.AppB-aarch64 (rec2)
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

func TestIterRecordsSkipsInvalid(t *testing.T) {
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

	records, err := IterRecords(tempDir)
	if err != nil {
		t.Fatalf("failed to iter records: %v", err)
	}

	// It should skip the invalid record, returning only 1 valid record
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	if records[0].Record.AppID != "org.example.AppValid" {
		t.Errorf("expected only valid record, got %s", records[0].Record.AppID)
	}
}
