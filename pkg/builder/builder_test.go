package builder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/config"
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

func TestBuildWithRemotesAndDependencies(t *testing.T) {
	mockExec := executil.NewMockExecutor()

	opts := BuildOptions{
		AppID:    "org.example.App",
		Manifest: "apps/org.example.App.json",
		Arch:     "x86_64",
		Branch:   "stable",
		StateDir: ".state",
		RepoPath: "repo",
		Remotes: map[string]config.RemoteConfig{
			"flathub": {URL: "https://dl.flathub.org/repo/flathub.flatpakrepo"},
		},
		Flatpaks: []config.FlatpakDep{
			{Remote: "flathub", Ref: "runtime/org.gnome.Platform/x86_64/45"},
			{Remote: "", Ref: "should-be-skipped"}, // empty remote should be skipped
		},
		Executor: mockExec,
	}

	err := Build(opts)
	if err != nil {
		t.Fatalf("expected build to succeed, got %v", err)
	}

	target := "--user"
	if os.Getuid() == 0 {
		target = "--system"
	}

	var remoteAddRan, installRan bool
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak" && len(cmd.Args) > 0 {
			if cmd.Args[0] == "remote-add" {
				remoteAddRan = true
				expectedArgs := []string{"remote-add", target, "--if-not-exists", "flathub", "https://dl.flathub.org/repo/flathub.flatpakrepo"}
				for i, arg := range expectedArgs {
					if cmd.Args[i] != arg {
						t.Errorf("unexpected arg at index %d for remote-add: got %q, expected %q", i, cmd.Args[i], arg)
					}
				}
			}
			if cmd.Args[0] == "install" {
				installRan = true
				expectedArgs := []string{"install", target, "-y", "flathub", "runtime/org.gnome.Platform/x86_64/45"}
				for i, arg := range expectedArgs {
					if cmd.Args[i] != arg {
						t.Errorf("unexpected arg at index %d for install: got %q, expected %q", i, cmd.Args[i], arg)
					}
				}
			}
		}
	}

	if !remoteAddRan {
		t.Error("expected flatpak remote-add command to have run")
	}
	if !installRan {
		t.Error("expected flatpak install command to have run")
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

func TestBuildPassesInstallFlag(t *testing.T) {
	mockExec := executil.NewMockExecutor()

	err := Build(BuildOptions{
		AppID: "org.example.App", Manifest: "m.json", Arch: "x86_64", Branch: "stable",
		StateDir: ".state", RepoPath: "repo",
		Install:  true,
		Executor: mockExec,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	var hasInstallCmd bool
	var installArgs []string
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak" && len(cmd.Args) > 0 && cmd.Args[0] == "install" {
			hasInstallCmd = true
			installArgs = cmd.Args
		}
	}

	if !hasInstallCmd {
		t.Fatal("expected flatpak install command to have run")
	}

	target := "--user"
	if os.Getuid() == 0 {
		target = "--system"
	}

	absRepoPath, err := filepath.Abs("repo")
	if err != nil {
		absRepoPath = "repo"
	}

	expectedArgs := []string{
		"install",
		target,
		"-y",
		"--reinstall",
		absRepoPath,
		"org.example.App",
	}

	if len(installArgs) != len(expectedArgs) {
		t.Fatalf("unexpected number of arguments: got %d, expected %d. Args: %v", len(installArgs), len(expectedArgs), installArgs)
	}

	for i, arg := range expectedArgs {
		if installArgs[i] != arg {
			t.Errorf("unexpected arg at index %d: got %q, expected %q", i, installArgs[i], arg)
		}
	}
}

func TestBuildWithBundle(t *testing.T) {
	mockExec := executil.NewMockExecutor()

	err := Build(BuildOptions{
		AppID:    "org.example.App",
		Manifest: "m.json",
		Arch:     "x86_64",
		Branch:   "stable",
		StateDir: ".state",
		RepoPath: "repo",
		Bundle:   true,
		Executor: mockExec,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	var hasBundleCmd bool
	var bundleArgs []string
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak" && len(cmd.Args) > 0 && cmd.Args[0] == "build-bundle" {
			hasBundleCmd = true
			bundleArgs = cmd.Args
		}
	}

	if !hasBundleCmd {
		t.Fatal("expected flatpak build-bundle command to have run")
	}

	expectedArgs := []string{
		"build-bundle",
		"--arch=x86_64",
		"repo",
		"org.example.App.flatpak",
		"org.example.App",
		"stable",
	}

	if len(bundleArgs) != len(expectedArgs) {
		t.Fatalf("unexpected number of arguments: got %d, expected %d. Args: %v", len(bundleArgs), len(expectedArgs), bundleArgs)
	}

	for i, arg := range expectedArgs {
		if bundleArgs[i] != arg {
			t.Errorf("unexpected arg at index %d: got %q, expected %q", i, bundleArgs[i], arg)
		}
	}
}

func TestBuildWithBundleResolvesFromRepo(t *testing.T) {
	mockExec := executil.NewMockExecutor()

	// Create a temp directory for the repo
	repoDir := t.TempDir()

	// Create refs/heads/app/org.example.App/x86_64/stable file
	refsDir := filepath.Join(repoDir, "refs", "heads", "app", "org.example.App", "x86_64")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refsDir, "stable"), []byte("commit-hash"), 0644); err != nil {
		t.Fatal(err)
	}

	err := Build(BuildOptions{
		Manifest: "m.json",
		StateDir: ".state",
		RepoPath: repoDir,
		Bundle:   true,
		Executor: mockExec,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	var hasBundleCmd bool
	var bundleArgs []string
	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak" && len(cmd.Args) > 0 && cmd.Args[0] == "build-bundle" {
			hasBundleCmd = true
			bundleArgs = cmd.Args
		}
	}

	if !hasBundleCmd {
		t.Fatal("expected flatpak build-bundle command to have run")
	}

	expectedArgs := []string{
		"build-bundle",
		"--arch=x86_64",
		repoDir,
		filepath.Join(filepath.Dir(repoDir), "org.example.App.flatpak"),
		"org.example.App",
		"stable",
	}

	if len(bundleArgs) != len(expectedArgs) {
		t.Fatalf("unexpected number of arguments: got %d, expected %d. Args: %v", len(bundleArgs), len(expectedArgs), bundleArgs)
	}

	for i, arg := range expectedArgs {
		if bundleArgs[i] != arg {
			t.Errorf("unexpected arg at index %d: got %q, expected %q", i, bundleArgs[i], arg)
		}
	}
}

func TestBuildLinterExceptionsAndDefaults(t *testing.T) {
	mockExec := executil.NewMockExecutor()
	mockExec.PathMap["flatpak-builder-lint"] = "/usr/bin/flatpak-builder-lint"

	// Create a temporary exceptions file to be read during build
	exceptionsFileContent := `{
		"org.example.App": ["app-specific-rule-1", "app-specific-rule-2"],
		"org.other.App": ["other-rule"],
		"*": ["wildcard-rule-1", "wildcard-rule-2"]
	}`
	tempFile, err := os.CreateTemp("", "test-exceptions-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())
	if _, err := tempFile.Write([]byte(exceptionsFileContent)); err != nil {
		t.Fatal(err)
	}
	tempFile.Close()

	var exceptionsData []byte
	mockExec.OnCommand = func(cmd *executil.MockCommand) {
		if cmd.Name == "flatpak-builder-lint" {
			// Find the "--user-exceptions" argument and read the file
			for i, arg := range cmd.Args {
				if arg == "--user-exceptions" && i+1 < len(cmd.Args) {
					path := cmd.Args[i+1]
					data, err := os.ReadFile(path)
					if err != nil {
						t.Errorf("failed to read linter exceptions file: %v", err)
						return
					}
					exceptionsData = data
				}
			}
		}
	}

	opts := BuildOptions{
		AppID:                "org.example.App",
		Manifest:             "apps/org.example.App.json",
		Arch:                 "x86_64",
		Branch:               "stable",
		StateDir:             ".state",
		RepoPath:             "repo",
		RunLinter:            true,
		LinterIgnoreRules:    []string{"inline-rule-1", "appstream-external-screenshot-url"}, // inline rule + one default duplicate
		LinterExceptions:     []string{"inline-exception-1", "wildcard-rule-1"},              // inline exception + one wildcard file duplicate
		LinterExceptionsFile: tempFile.Name(),
		Executor:             mockExec,
	}

	err = Build(opts)
	if err != nil {
		t.Fatalf("expected build to succeed, got %v", err)
	}

	if len(exceptionsData) == 0 {
		t.Fatal("expected exceptions JSON file to be generated and read, but got empty data")
	}

	var parsed map[string][]string
	if err := json.Unmarshal(exceptionsData, &parsed); err != nil {
		t.Fatalf("failed to parse generated exceptions JSON: %v", err)
	}

	expectedRules := []string{
		"appstream-external-screenshot-url",
		"appstream-screenshots-not-mirrored-in-ostree",
		"inline-rule-1",
		"inline-exception-1",
		"app-specific-rule-1",
		"app-specific-rule-2",
		"wildcard-rule-1",
		"wildcard-rule-2",
	}

	rulesForApp, exists := parsed["org.example.App"]
	if !exists {
		t.Errorf("expected key %q in parsed exceptions", opts.AppID)
	}

	if _, existsWildcard := parsed["*"]; existsWildcard {
		t.Error("expected no wildcard '*' key in parsed exceptions when AppID is specified")
	}

	contains := func(slice []string, val string) bool {
		for _, item := range slice {
			if item == val {
				return true
			}
		}
		return false
	}

	for _, expected := range expectedRules {
		if !contains(rulesForApp, expected) {
			t.Errorf("expected rule %q to be in exceptions, but not found", expected)
		}
	}

	if len(rulesForApp) != len(expectedRules) {
		t.Errorf("expected %d rules, but got %d: %v", len(expectedRules), len(rulesForApp), rulesForApp)
	}

	if contains(rulesForApp, "other-rule") {
		t.Errorf("rules for %q should not contain other-rule", opts.AppID)
	}
}

func TestBuildLinterDefaultsOnly(t *testing.T) {
	mockExec := executil.NewMockExecutor()
	mockExec.PathMap["flatpak-builder-lint"] = "/usr/bin/flatpak-builder-lint"

	var exceptionsData []byte
	mockExec.OnCommand = func(cmd *executil.MockCommand) {
		if cmd.Name == "flatpak-builder-lint" {
			for i, arg := range cmd.Args {
				if arg == "--user-exceptions" && i+1 < len(cmd.Args) {
					path := cmd.Args[i+1]
					data, err := os.ReadFile(path)
					if err != nil {
						t.Errorf("failed to read linter exceptions file: %v", err)
						return
					}
					exceptionsData = data
				}
			}
		}
	}

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

	if len(exceptionsData) == 0 {
		t.Fatal("expected default exceptions JSON file to be generated, but got empty data")
	}

	var parsed map[string][]string
	if err := json.Unmarshal(exceptionsData, &parsed); err != nil {
		t.Fatalf("failed to parse generated exceptions JSON: %v", err)
	}

	expectedRules := []string{
		"appstream-external-screenshot-url",
		"appstream-screenshots-not-mirrored-in-ostree",
	}

	rulesForApp, exists := parsed["org.example.App"]
	if !exists {
		t.Fatalf("expected key %q in parsed exceptions", opts.AppID)
	}

	for _, expected := range expectedRules {
		found := false
		for _, r := range rulesForApp {
			if r == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected default rule %q to be in exceptions, but not found", expected)
		}
	}

	if len(rulesForApp) != len(expectedRules) {
		t.Errorf("expected exactly %d rules, but got %d: %v", len(expectedRules), len(rulesForApp), rulesForApp)
	}
}

func TestBuildLinterExceptionsFileNotFound(t *testing.T) {
	mockExec := executil.NewMockExecutor()

	opts := BuildOptions{
		AppID:                "org.example.App",
		Manifest:             "apps/org.example.App.json",
		LinterExceptionsFile: "/non/existent/path/exceptions.json",
		Executor:             mockExec,
	}

	err := Build(opts)
	if err == nil {
		t.Fatal("expected error when linter exceptions file does not exist, but got nil")
	}
	if !strings.Contains(err.Error(), "failed to read linter exceptions file") {
		t.Errorf("expected file read error message, got: %v", err)
	}
}

func TestBuildWithExplodedRemotes(t *testing.T) {
	mockExec := executil.NewMockExecutor()

	falseVal := false
	trueVal := true
	inlineGPGKey := `-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: GnuPG v2

mQENBFT9...
-----END PGP PUBLIC KEY BLOCK-----`

	opts := BuildOptions{
		AppID:    "org.example.App",
		Manifest: "apps/org.example.App.json",
		Arch:     "x86_64",
		Branch:   "stable",
		StateDir: ".state",
		RepoPath: "repo",
		Remotes: map[string]config.RemoteConfig{
			"remote-no-gpg": {
				URL:       "https://example.com/no-gpg.flatpakrepo",
				GPGVerify: &falseVal,
			},
			"remote-gpg-key": {
				URL:          "https://example.com/with-gpg.flatpakrepo",
				GPGVerify:    &trueVal,
				GPGKey:       inlineGPGKey,
				SigVerifyURL: "https://example.com/sigs",
			},
		},
		Executor: mockExec,
	}

	err := Build(opts)
	if err != nil {
		t.Fatalf("expected build to succeed, got %v", err)
	}

	// Verify command-line arguments for flatpak remote-add and remote-modify
	var noGpgAddRan, noGpgModifyRan bool
	var gpgKeyAddRan, gpgKeyModifyRan bool

	for _, cmd := range mockExec.Commands {
		if cmd.Name == "flatpak" && len(cmd.Args) > 0 {
			argsJoin := strings.Join(cmd.Args, " ")
			if cmd.Args[0] == "remote-add" {
				if strings.Contains(argsJoin, "remote-no-gpg") {
					noGpgAddRan = true
					if !strings.Contains(argsJoin, "--no-gpg-verify") {
						t.Errorf("expected --no-gpg-verify in remote-add args: %v", cmd.Args)
					}
				}
				if strings.Contains(argsJoin, "remote-gpg-key") {
					gpgKeyAddRan = true
					if strings.Contains(argsJoin, "--no-gpg-verify") {
						t.Errorf("expected no --no-gpg-verify in remote-add args: %v", cmd.Args)
					}
					if !strings.Contains(argsJoin, "--gpg-import=") {
						t.Errorf("expected --gpg-import in remote-add args: %v", cmd.Args)
					}
					if !strings.Contains(argsJoin, "--signature-lookaside=https://example.com/sigs") {
						t.Errorf("expected --signature-lookaside in remote-add args: %v", cmd.Args)
					}
				}
			}
			if cmd.Args[0] == "remote-modify" {
				if strings.Contains(argsJoin, "remote-no-gpg") {
					noGpgModifyRan = true
					if !strings.Contains(argsJoin, "--no-gpg-verify") {
						t.Errorf("expected --no-gpg-verify in remote-modify args: %v", cmd.Args)
					}
				}
				if strings.Contains(argsJoin, "remote-gpg-key") {
					gpgKeyModifyRan = true
					if !strings.Contains(argsJoin, "--gpg-verify") {
						t.Errorf("expected --gpg-verify in remote-modify args: %v", cmd.Args)
					}
					if !strings.Contains(argsJoin, "--gpg-import=") {
						t.Errorf("expected --gpg-import in remote-modify args: %v", cmd.Args)
					}
					if !strings.Contains(argsJoin, "--signature-lookaside=https://example.com/sigs") {
						t.Errorf("expected --signature-lookaside in remote-modify args: %v", cmd.Args)
					}
				}
			}
		}
	}

	if !noGpgAddRan {
		t.Error("expected remote-add for remote-no-gpg to have run")
	}
	if !noGpgModifyRan {
		t.Error("expected remote-modify for remote-no-gpg to have run")
	}
	if !gpgKeyAddRan {
		t.Error("expected remote-add for remote-gpg-key to have run")
	}
	if !gpgKeyModifyRan {
		t.Error("expected remote-modify for remote-gpg-key to have run")
	}
}

func TestBuildAutoInjectsInstallDepsFrom(t *testing.T) {
	cases := []struct {
		name          string
		noInstallDeps bool
		noFlathub     bool
		remotes       map[string]config.RemoteConfig
		builderArgs   []string
		wantArgs      []string
		dontWantArgs  []string
		wantRemotes   []string
	}{
		{
			name:        "default auto-injects flathub",
			wantArgs:    []string{"--install-deps-from=flathub"},
			wantRemotes: []string{"flathub"},
		},
		{
			name:          "no-install-deps disables all auto-injections",
			noInstallDeps: true,
			dontWantArgs:  []string{"--install-deps-from=flathub"},
			wantRemotes:   []string{"flathub"}, // Flathub is still auto-registered as a remote
		},
		{
			name:         "no-flathub disables flathub remote and flathub auto-inject",
			noFlathub:    true,
			dontWantArgs: []string{"--install-deps-from=flathub"},
			wantRemotes:  []string{}, // No flathub remote registered
		},
		{
			name: "auto-injects custom remotes and flathub",
			remotes: map[string]config.RemoteConfig{
				"custom-repo": {URL: "https://example.com/repo"},
			},
			wantArgs:    []string{"--install-deps-from=flathub", "--install-deps-from=custom-repo"},
			wantRemotes: []string{"flathub", "custom-repo"},
		},
		{
			name: "custom flathub override is preserved",
			remotes: map[string]config.RemoteConfig{
				"flathub": {URL: "https://custom.flathub.org/repo"},
			},
			wantArgs:    []string{"--install-deps-from=flathub"},
			wantRemotes: []string{"flathub"},
		},
		{
			name:        "no duplicates when explicitly set in builder args",
			builderArgs: []string{"--install-deps-from=flathub"},
			wantArgs:    []string{"--install-deps-from=flathub"}, // check that it is not added twice
			wantRemotes: []string{"flathub"},
		},
		{
			name:          "explicit user --install-deps-from=custom-remote is preserved even with no-install-deps",
			noInstallDeps: true,
			builderArgs:   []string{"--install-deps-from=custom-remote"},
			wantArgs:      []string{"--install-deps-from=custom-remote"},
			dontWantArgs:  []string{"--install-deps-from=flathub"},
			wantRemotes:   []string{"flathub"},
		},
		{
			name:        "explicit user other args are preserved",
			builderArgs: []string{"--jobs=4"},
			wantArgs:    []string{"--jobs=4", "--install-deps-from=flathub"},
			wantRemotes: []string{"flathub"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockExec := executil.NewMockExecutor()

			opts := BuildOptions{
				AppID:         "org.example.App",
				Manifest:      "apps/org.example.App.json",
				StateDir:      ".state",
				RepoPath:      "repo",
				NoInstallDeps: tc.noInstallDeps,
				NoFlathub:     tc.noFlathub,
				Remotes:       tc.remotes,
				BuilderArgs:   tc.builderArgs,
				Executor:      mockExec,
			}

			err := Build(opts)
			if err != nil {
				t.Fatalf("build failed: %v", err)
			}

			// Find flatpak-builder command and verify args
			var builderArgs []string
			for _, cmd := range mockExec.Commands {
				if cmd.Name == "flatpak-builder" {
					builderArgs = cmd.Args
					break
				}
			}

			if len(builderArgs) == 0 {
				t.Fatalf("flatpak-builder did not run")
			}

			// Helper to check if slice contains arg
			contains := func(slice []string, val string) bool {
				for _, item := range slice {
					if item == val {
						return true
					}
				}
				return false
			}

			for _, want := range tc.wantArgs {
				if !contains(builderArgs, want) {
					t.Errorf("expected argument %q not found. Got: %v", want, builderArgs)
				}
			}

			for _, dontWant := range tc.dontWantArgs {
				if contains(builderArgs, dontWant) {
					t.Errorf("unexpected argument %q found. Got: %v", dontWant, builderArgs)
				}
			}

			// Verify registered remotes
			var registeredRemotes []string
			for _, cmd := range mockExec.Commands {
				if cmd.Name == "flatpak" && len(cmd.Args) > 0 && cmd.Args[0] == "remote-add" {
					name := cmd.Args[len(cmd.Args)-2]
					registeredRemotes = append(registeredRemotes, name)
				}
			}

			// Check all expected remotes registered
			for _, wantRem := range tc.wantRemotes {
				if !contains(registeredRemotes, wantRem) {
					t.Errorf("expected remote %q to be registered. Registered remotes: %v", wantRem, registeredRemotes)
				}
			}

			// Check no unexpected remotes registered
			if len(tc.wantRemotes) == 0 && len(registeredRemotes) > 0 {
				t.Errorf("expected no remotes registered, but got: %v", registeredRemotes)
			}
		})
	}
}
