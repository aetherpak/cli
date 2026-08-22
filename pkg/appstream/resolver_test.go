package appstream

import (
	"os"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
)

func TestResolveRelease_Explicit(t *testing.T) {
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
	os.Setenv("GITHUB_REF_TYPE", "tag")
	os.Setenv("GITHUB_REF_NAME", "v2.0.0-beta.1")
	defer func() {
		os.Unsetenv("GITHUB_REF_TYPE")
		os.Unsetenv("GITHUB_REF_NAME")
	}()

	res, ok := ResolveRelease(nil, "", "", "", "")
	if !ok {
		t.Fatalf("expected ok=true for GITHUB_REF_NAME")
	}
	if res.Version != "2.0.0-beta.1" {
		t.Errorf("expected version 2.0.0-beta.1, got %s", res.Version)
	}
	if res.Source != "env:GITHUB_REF_NAME" {
		t.Errorf("expected source env:GITHUB_REF_NAME, got %s", res.Source)
	}
	if res.Date == "" {
		t.Errorf("expected non-empty date")
	}
}

func TestResolveRelease_GitMock(t *testing.T) {
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
}
