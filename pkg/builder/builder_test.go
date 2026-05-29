package builder

import (
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
)

func TestResolveLinterCmd(t *testing.T) {
	mockExec := executil.NewMockExecutor()
	mockExec.PathMap["flatpak-builder-lint"] = "/usr/bin/flatpak-builder-lint"

	cmd, _ := resolveLinterCmd(mockExec)
	if cmd != "flatpak-builder-lint" {
		t.Errorf("expected flatpak-builder-lint when in path, got %s", cmd)
	}

	mockExec2 := executil.NewMockExecutor() // no path registered
	cmd, args := resolveLinterCmd(mockExec2)
	if cmd != "flatpak" {
		t.Errorf("expected flatpak when linter not in path, got %s", cmd)
	}
	if len(args) < 3 || args[0] != "run" {
		t.Errorf("expected flatpak run args, got %v", args)
	}
}

func TestBuild(t *testing.T) {
	mockExec := executil.NewMockExecutor()
	mockExec.PathMap["flatpak-builder-lint"] = "/usr/bin/flatpak-builder-lint"

	opts := BuildOptions{
		AppID:     "org.example.App",
		Manifest:  "apps/org.example.App.json",
		Arch:      "x86_64",
		Branch:    "stable",
		StateDir:  ".state",
		RepoPath:  "repo",
		RunLinter: true,
		Executor:  mockExec,
	}

	err := Build(opts)
	if err != nil {
		t.Fatalf("expected build to succeed, got %v", err)
	}

	// Verify the flatpak-builder command was executed
	var builderRan, linterRan bool
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak-builder" {
			builderRan = true
			// Verify arguments
			var hasClean, hasRepo, hasArch, hasBranch bool
			for _, arg := range cmd.Args {
				if arg == "--force-clean" {
					hasClean = true
				}
				if arg == "--repo=repo" {
					hasRepo = true
				}
				if arg == "--arch=x86_64" {
					hasArch = true
				}
				if arg == "--default-branch=stable" {
					hasBranch = true
				}
			}
			if !hasClean || !hasRepo || !hasArch || !hasBranch {
				t.Errorf("missing expected flatpak-builder argument. Args: %v", cmd.Args)
			}
		}
		if cmd.Name == "flatpak-builder-lint" {
			linterRan = true
		}
	}

	if !builderRan {
		t.Errorf("expected flatpak-builder to have run")
	}
	if !linterRan {
		t.Errorf("expected flatpak-builder-lint to have run")
	}
}
