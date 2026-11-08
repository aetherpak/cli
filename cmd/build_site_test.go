package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSiteSeedFlagsConflict(t *testing.T) {
	names := []string{"index-file", "seed-dir", "site-dir", "records-dir", "no-sign"}
	type flagState struct {
		changed bool
		value   string
	}
	saved := make(map[string]flagState, len(names))
	for _, name := range names {
		f := buildSiteCmd.Flags().Lookup(name)
		saved[name] = flagState{changed: f.Changed, value: f.Value.String()}
	}
	oldIndex, oldSeed, oldSiteDir := siteIndexFile, siteSeedDir, siteDir
	oldRecordsDir, oldNoSign := siteRecordsDir, siteNoSign
	defer func() {
		siteIndexFile, siteSeedDir, siteDir = oldIndex, oldSeed, oldSiteDir
		siteRecordsDir, siteNoSign = oldRecordsDir, oldNoSign
		for _, name := range names {
			f := buildSiteCmd.Flags().Lookup(name)
			f.Changed = saved[name].changed
			_ = f.Value.Set(saved[name].value)
		}
	}()

	tempDir := t.TempDir()
	for name, value := range map[string]string{
		"index-file":  "index/static",
		"seed-dir":    "site",
		"site-dir":    tempDir, // keep a regressed guard out of the checkout
		"records-dir": filepath.Join(tempDir, "records"),
		"no-sign":     "true",
	} {
		if err := buildSiteCmd.Flags().Set(name, value); err != nil {
			t.Fatalf("failed to set --%s: %v", name, err)
		}
	}

	err := buildSiteCmd.RunE(buildSiteCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--index-file and --seed-dir are mutually exclusive") {
		t.Fatalf("RunE error = %v, want the seed flag conflict", err)
	}
}

func TestValidateSeedSource(t *testing.T) {
	tests := []struct {
		name      string
		indexFile string
		seedDir   string
		wantErr   bool
	}{
		{name: "no local seed"},
		{name: "index file only", indexFile: "/tmp/site/index/static"},
		{name: "seed dir only", seedDir: "/tmp/site"},
		{name: "both local seeds", indexFile: "/tmp/site/index/static", seedDir: "/tmp/site", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSeedSource(tt.indexFile, tt.seedDir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSeedSource() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "mutually exclusive") {
				t.Errorf("error = %q, want it to name the conflict", err)
			}
		})
	}
}
