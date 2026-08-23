package appstream

import (
	"errors"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
)

var ciTagEnvVars = []string{
	"GITHUB_REF_TYPE",
	"GITHUB_REF_NAME",
	"GITHUB_REF",
	"CI_COMMIT_TAG",
	"CIRCLE_TAG",
	"TRAVIS_TAG",
}

func clearCIEnv(t *testing.T) {
	t.Helper()
	for _, k := range ciTagEnvVars {
		t.Setenv(k, "")
	}
}

func TestResolveRelease_Explicit(t *testing.T) {
	clearCIEnv(t)

	res, ok := ResolveRelease(nil, "v1.5.0", "2026-08-22", "Awesome release", "https://example.com")
	if !ok {
		t.Fatalf("expected ok=true for explicit version")
	}
	if res.Version != "1.5.0" {
		t.Errorf("expected version 1.5.0, got %s", res.Version)
	}
	if res.Date != "2026-08-22" {
		t.Errorf("expected date 2026-08-22, got %s", res.Date)
	}
	if res.Description != "Awesome release" {
		t.Errorf("expected description match, got %s", res.Description)
	}
	if res.URL != "https://example.com" {
		t.Errorf("expected url match, got %s", res.URL)
	}
	if res.Source != "explicit" {
		t.Errorf("expected source explicit, got %s", res.Source)
	}
}

func TestResolveRelease_CIEnv(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T)
		expectedVer string
		expectedSrc string
	}{
		{
			name: "GITHUB_REF_NAME",
			setup: func(t *testing.T) {
				t.Setenv("GITHUB_REF_TYPE", "tag")
				t.Setenv("GITHUB_REF_NAME", "v2.0.0-beta.1")
			},
			expectedVer: "2.0.0-beta.1",
			expectedSrc: "env:GITHUB_REF_NAME",
		},
		{
			name: "GITHUB_REF",
			setup: func(t *testing.T) {
				t.Setenv("GITHUB_REF", "refs/tags/v2.1.0")
			},
			expectedVer: "2.1.0",
			expectedSrc: "env:GITHUB_REF",
		},
		{
			name: "CI_COMMIT_TAG",
			setup: func(t *testing.T) {
				t.Setenv("CI_COMMIT_TAG", "v2.2.0")
			},
			expectedVer: "2.2.0",
			expectedSrc: "env:CI_COMMIT_TAG",
		},
		{
			name: "CIRCLE_TAG",
			setup: func(t *testing.T) {
				t.Setenv("CIRCLE_TAG", "v2.3.0")
			},
			expectedVer: "2.3.0",
			expectedSrc: "env:CIRCLE_TAG",
		},
		{
			name: "TRAVIS_TAG",
			setup: func(t *testing.T) {
				t.Setenv("TRAVIS_TAG", "v2.4.0")
			},
			expectedVer: "2.4.0",
			expectedSrc: "env:TRAVIS_TAG",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearCIEnv(t)
			tt.setup(t)

			res, ok := ResolveRelease(nil, "", "", "", "")
			if !ok {
				t.Fatalf("expected ok=true")
			}
			if res.Version != tt.expectedVer {
				t.Errorf("expected version %s, got %s", tt.expectedVer, res.Version)
			}
			if res.Source != tt.expectedSrc {
				t.Errorf("expected source %s, got %s", tt.expectedSrc, res.Source)
			}
			if res.Date == "" {
				t.Errorf("expected non-empty date")
			}
		})
	}
}

func TestResolveRelease_GitMock(t *testing.T) {
	t.Run("ExactTag", func(t *testing.T) {
		clearCIEnv(t)
		mockExec := executil.NewMockExecutor()
		mockExec.OnCommand = func(cmd *executil.MockCommand) {
			if cmd.Name == "git" && len(cmd.Args) >= 3 && cmd.Args[0] == "describe" && cmd.Args[1] == "--tags" && cmd.Args[2] == "--exact-match" {
				cmd.OutData = []byte("v3.2.1\n")
			}
			if cmd.Name == "git" && len(cmd.Args) >= 4 && cmd.Args[0] == "log" && cmd.Args[2] == "--format=%cs" {
				cmd.OutData = []byte("2026-05-10\n")
			}
		}

		res, ok := ResolveRelease(mockExec, "", "", "", "")
		if !ok {
			t.Fatalf("expected ok=true for git mock tag")
		}
		if res.Version != "3.2.1" {
			t.Errorf("expected version 3.2.1, got %s", res.Version)
		}
		if res.Date != "2026-05-10" {
			t.Errorf("expected date 2026-05-10 from git log, got %s", res.Date)
		}
		if res.Source != "git:exact-tag" {
			t.Errorf("expected source git:exact-tag, got %s", res.Source)
		}
	})

	t.Run("DescribeTagFallback", func(t *testing.T) {
		clearCIEnv(t)
		mockExec := executil.NewMockExecutor()
		mockExec.OnCommand = func(cmd *executil.MockCommand) {
			if cmd.Name == "git" && len(cmd.Args) >= 3 && cmd.Args[0] == "describe" && cmd.Args[1] == "--tags" && cmd.Args[2] == "--exact-match" {
				cmd.RunErr = errors.New("tag not found")
			}
			if cmd.Name == "git" && len(cmd.Args) >= 3 && cmd.Args[0] == "describe" && cmd.Args[1] == "--tags" && cmd.Args[2] == "--abbrev=0" {
				cmd.OutData = []byte("v3.2.0\n")
			}
			if cmd.Name == "git" && len(cmd.Args) >= 4 && cmd.Args[0] == "log" && cmd.Args[2] == "--format=%cs" {
				cmd.OutData = []byte("2026-05-01\n")
			}
		}

		res, ok := ResolveRelease(mockExec, "", "", "", "")
		if !ok {
			t.Fatalf("expected ok=true for git describe tag fallback")
		}
		if res.Version != "3.2.0" {
			t.Errorf("expected version 3.2.0, got %s", res.Version)
		}
		if res.Date != "2026-05-01" {
			t.Errorf("expected date 2026-05-01 from git log, got %s", res.Date)
		}
		if res.Source != "git:describe-tag" {
			t.Errorf("expected source git:describe-tag, got %s", res.Source)
		}
	})

	t.Run("NoTagFound", func(t *testing.T) {
		clearCIEnv(t)
		mockExec := executil.NewMockExecutor()
		mockExec.OnCommand = func(cmd *executil.MockCommand) {
			cmd.RunErr = errors.New("no tag")
		}

		_, ok := ResolveRelease(mockExec, "", "", "", "")
		if ok {
			t.Fatalf("expected ok=false when no tags found")
		}
	})

	t.Run("ExplicitDateOverridesGitDate", func(t *testing.T) {
		clearCIEnv(t)
		mockExec := executil.NewMockExecutor()
		mockExec.OnCommand = func(cmd *executil.MockCommand) {
			if cmd.Name == "git" && len(cmd.Args) >= 3 && cmd.Args[0] == "describe" && cmd.Args[1] == "--tags" && cmd.Args[2] == "--exact-match" {
				cmd.OutData = []byte("v3.2.1\n")
			}
			if cmd.Name == "git" && len(cmd.Args) >= 4 && cmd.Args[0] == "log" && cmd.Args[2] == "--format=%cs" {
				cmd.OutData = []byte("2026-05-10\n")
			}
		}

		res, ok := ResolveRelease(mockExec, "", "2026-12-31", "", "")
		if !ok {
			t.Fatalf("expected ok=true")
		}
		if res.Date != "2026-12-31" {
			t.Errorf("expected explicit date 2026-12-31, got %s", res.Date)
		}
	})
}
