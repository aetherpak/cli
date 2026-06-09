package plan

import (
	"strings"
	"testing"
)

func TestGenerateGitLabPipelineEmpty(t *testing.T) {
	res := &PlanResult{}
	got := GenerateGitLabPipeline(res, false)
	if !strings.Contains(got, "no-op:") {
		t.Errorf("expected no-op job for empty plan, got:\n%s", got)
	}
}

func TestGenerateGitLabPipeline(t *testing.T) {
	res := &PlanResult{
		MatrixManifest: []MatrixRow{
			{
				AppID:    "org.example.App",
				Manifest: "apps/org.example.App/manifest.yaml",
				Arch:     "x86_64",
				Branch:   "stable",
			},
			{
				AppID:    "org.example.App",
				Manifest: "apps/org.example.App/manifest.yaml",
				Arch:     "aarch64",
				Branch:   "stable",
			},
		},
	}

	// 1. Without arm64
	got := GenerateGitLabPipeline(res, false)
	if !strings.Contains(got, "build:org_example_App-x86_64") {
		t.Errorf("expected build:org_example_App-x86_64 job, got:\n%s", got)
	}
	if strings.Contains(got, "build:org_example_App-aarch64") {
		t.Errorf("expected build:org_example_App-aarch64 job to be filtered out, got:\n%s", got)
	}

	// 2. With arm64
	gotArm := GenerateGitLabPipeline(res, true)
	if !strings.Contains(gotArm, "build:org_example_App-x86_64") {
		t.Errorf("expected build:org_example_App-x86_64 job, got:\n%s", gotArm)
	}
	if !strings.Contains(gotArm, "build:org_example_App-aarch64") {
		t.Errorf("expected build:org_example_App-aarch64 job, got:\n%s", gotArm)
	}
	if !strings.Contains(gotArm, "$ARM_RUNNER_TAG") {
		t.Errorf("expected arm runner tag, got:\n%s", gotArm)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		in, out string
	}{
		{"org.gnome.Sudoku", "org_gnome_Sudoku"},
		{"com.example-app", "com_example_app"},
		{"a123_BC", "a123_BC"},
	}
	for _, tc := range tests {
		if got := slugify(tc.in); got != tc.out {
			t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}
