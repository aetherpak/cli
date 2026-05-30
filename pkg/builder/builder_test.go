package builder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
)

// writeGitmodules writes a .gitmodules declaring one submodule at the given
// relative path inside dir.
func writeGitmodules(t *testing.T, dir, submodulePath string) {
	t.Helper()
	content := "[submodule \"" + submodulePath + "\"]\n\tpath = " + submodulePath +
		"\n\turl = https://example.invalid/" + submodulePath + ".git\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitmodules"), []byte(content), 0o644); err != nil {
		t.Fatalf("write .gitmodules: %v", err)
	}
}

func TestCheckSubmodulesErrorsOnUninitialized(t *testing.T) {
	appDir := t.TempDir()
	writeGitmodules(t, appDir, "shared-modules")
	// Uninitialized submodule = empty directory.
	if err := os.Mkdir(filepath.Join(appDir, "shared-modules"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := checkSubmodules(filepath.Join(appDir, "manifest.json"))
	if err == nil {
		t.Fatalf("expected error for uninitialized submodule")
	}
	if !strings.Contains(err.Error(), "shared-modules") {
		t.Errorf("error should name the submodule, got: %v", err)
	}
	if !strings.Contains(err.Error(), "git submodule update --init --recursive") {
		t.Errorf("error should tell the user how to fix it, got: %v", err)
	}
}

func TestCheckSubmodulesPassesWhenPopulated(t *testing.T) {
	appDir := t.TempDir()
	writeGitmodules(t, appDir, "shared-modules")
	sub := filepath.Join(appDir, "shared-modules")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "module.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := checkSubmodules(filepath.Join(appDir, "manifest.json")); err != nil {
		t.Fatalf("expected no error when submodule is populated, got: %v", err)
	}
}

func TestCheckSubmodulesNoGitmodules(t *testing.T) {
	appDir := t.TempDir()
	if err := checkSubmodules(filepath.Join(appDir, "manifest.json")); err != nil {
		t.Fatalf("expected no error without .gitmodules, got: %v", err)
	}
}

func TestCheckSubmodulesDetectsNested(t *testing.T) {
	appDir := t.TempDir()
	writeGitmodules(t, appDir, "shared-modules")
	shared := filepath.Join(appDir, "shared-modules")
	if err := os.Mkdir(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	// shared-modules is populated but declares its own uninitialized submodule.
	writeGitmodules(t, shared, "nested")
	if err := os.Mkdir(filepath.Join(shared, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := checkSubmodules(filepath.Join(appDir, "manifest.json"))
	if err == nil || !strings.Contains(err.Error(), filepath.Join("shared-modules", "nested")) {
		t.Fatalf("expected nested submodule to be reported, got: %v", err)
	}
}

func TestBuildFailsOnUninitializedSubmodule(t *testing.T) {
	appDir := t.TempDir()
	writeGitmodules(t, appDir, "shared-modules")
	if err := os.Mkdir(filepath.Join(appDir, "shared-modules"), 0o755); err != nil {
		t.Fatal(err)
	}

	mockExec := executil.NewMockExecutor()
	err := Build(BuildOptions{
		AppID: "org.example.App", Manifest: filepath.Join(appDir, "manifest.json"),
		Arch: "x86_64", Branch: "stable", StateDir: ".state", RepoPath: "repo",
		Executor: mockExec,
	})
	if err == nil {
		t.Fatalf("build must fail when a required submodule is uninitialized")
	}

	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak-builder" {
			t.Errorf("flatpak-builder must not run when submodules are uninitialized")
		}
	}
}

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

func TestBuildOmitEmptyFlags(t *testing.T) {
	mockExec := executil.NewMockExecutor()

	opts := BuildOptions{
		AppID:    "org.example.App",
		Manifest: "apps/org.example.App.json",
		Arch:     "",
		Branch:   "",
		StateDir: ".state",
		RepoPath: "repo",
		Executor: mockExec,
	}

	err := Build(opts)
	if err != nil {
		t.Fatalf("expected build to succeed, got %v", err)
	}

	var builderRan bool
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak-builder" {
			builderRan = true
			for _, arg := range cmd.Args {
				if strings.HasPrefix(arg, "--arch=") {
					t.Errorf("found --arch= flag, expected it to be omitted when empty: %s", arg)
				}
				if strings.HasPrefix(arg, "--default-branch=") {
					t.Errorf("found --default-branch= flag, expected it to be omitted when empty: %s", arg)
				}
			}
		}
	}

	if !builderRan {
		t.Errorf("expected flatpak-builder to have run")
	}
}

func TestExtraBuilderArgs(t *testing.T) {
	cases := []struct {
		name     string
		passthru []string
		ciEnv    string
		want     []string
	}{
		{"no ci, no passthru", nil, "", nil},
		{"ci disables rofiles-fuse", nil, "true", []string{"--disable-rofiles-fuse"}},
		{"passthru preserved, ci appends", []string{"--jobs=4"}, "true", []string{"--jobs=4", "--disable-rofiles-fuse"}},
		{"no dup when already set", []string{"--disable-rofiles-fuse"}, "true", []string{"--disable-rofiles-fuse"}},
		{"no ci keeps passthru only", []string{"--jobs=4"}, "", []string{"--jobs=4"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extraBuilderArgs(tc.passthru, tc.ciEnv)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
}

func TestBuildPassesBuilderArgs(t *testing.T) {
	t.Setenv("CI", "true")
	mockExec := executil.NewMockExecutor()

	err := Build(BuildOptions{
		AppID: "org.example.App", Manifest: "m.json", Arch: "x86_64", Branch: "stable",
		StateDir: ".state", RepoPath: "repo",
		BuilderArgs: []string{"--jobs=2"},
		Executor:    mockExec,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	var args []string
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak-builder" {
			args = cmd.Args
		}
	}
	mhas := func(want string) bool {
		for _, a := range args {
			if a == want {
				return true
			}
		}
		return false
	}
	if !mhas("--jobs=2") {
		t.Errorf("passthrough --jobs=2 missing: %v", args)
	}
	if !mhas("--disable-rofiles-fuse") {
		t.Errorf("CI should add --disable-rofiles-fuse: %v", args)
	}
	// Positional builddir + manifest must remain last.
	if args[len(args)-1] != "m.json" {
		t.Errorf("manifest must be the final arg: %v", args)
	}
}
