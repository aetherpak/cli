package ciout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmitKeyValidation(t *testing.T) {
	tempDir := t.TempDir()
	outPath := filepath.Join(tempDir, "ci_output")

	tests := []struct {
		name    string
		pairs   []KV
		wantErr bool
	}{
		{
			name: "valid keys",
			pairs: []KV{
				{Key: "APP_ID", Value: "org.example.App"},
				{Key: "branch-name", Value: "stable"},
				{Key: "arch_123", Value: "x86_64"},
			},
			wantErr: false,
		},
		{
			name: "invalid key containing equals",
			pairs: []KV{
				{Key: "APP=ID", Value: "org.example.App"},
			},
			wantErr: true,
		},
		{
			name: "invalid key containing newline",
			pairs: []KV{
				{Key: "APP\nID", Value: "org.example.App"},
			},
			wantErr: true,
		},
		{
			name: "invalid key containing space",
			pairs: []KV{
				{Key: "APP ID", Value: "org.example.App"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Emit(outPath, tt.pairs)
			if (err != nil) != tt.wantErr {
				t.Errorf("Emit() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil {
				// Clean up generated file
				os.Remove(outPath)
			}
		})
	}
}
