package gitutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
)

func TestSubmoduleAddArgs(t *testing.T) {
	mock := executil.NewMockExecutor()
	g := NewWithExecutor(mock)

	if err := g.SubmoduleAdd("https://example.com/repo.git", "sources/org.example.App"); err != nil {
		t.Fatalf("SubmoduleAdd: %v", err)
	}
	if len(mock.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(mock.Commands))
	}
	c := mock.Commands[0]
	want := []string{"-c", "safe.directory=*", "submodule", "add", "https://example.com/repo.git", "sources/org.example.App"}
	if c.Name != "git" || !equal(c.Args, want) {
		t.Errorf("got git %v, want git %v", c.Args, want)
	}
}

func TestSubmoduleUpdateInitRecursive(t *testing.T) {
	mock := executil.NewMockExecutor()
	g := NewWithExecutor(mock)
	if err := g.SubmoduleUpdateInit(true); err != nil {
		t.Fatalf("SubmoduleUpdateInit: %v", err)
	}
	want := []string{"-c", "safe.directory=*", "submodule", "update", "--init", "--recursive"}
	if !equal(mock.Commands[0].Args, want) {
		t.Errorf("got %v, want %v", mock.Commands[0].Args, want)
	}
}

func TestSubmoduleRemoveSequence(t *testing.T) {
	mock := executil.NewMockExecutor()
	// rev-parse --git-common-dir resolves the module store root; point it at a
	// temp dir so the filesystem removal step is exercised harmlessly.
	mock.OutMap["git"] = []byte(t.TempDir() + "\n")
	g := NewWithExecutor(mock)
	if err := g.SubmoduleRemove("sources/org.example.App"); err != nil {
		t.Fatalf("SubmoduleRemove: %v", err)
	}
	gotCmds := [][]string{}
	for _, c := range mock.Commands {
		gotCmds = append(gotCmds, c.Args)
	}
	// Steps 1-2 are git commands; step 3 resolves the module store via
	// rev-parse, then removes it from the filesystem (no git command).
	wantCmds := [][]string{
		{"-c", "safe.directory=*", "submodule", "deinit", "-f", "sources/org.example.App"},
		{"-c", "safe.directory=*", "rm", "-f", "sources/org.example.App"},
		{"-c", "safe.directory=*", "rev-parse", "--git-common-dir"},
	}
	if len(gotCmds) != len(wantCmds) {
		t.Fatalf("got %d commands, want %d: %v", len(gotCmds), len(wantCmds), gotCmds)
	}
	for i := range wantCmds {
		if !equal(gotCmds[i], wantCmds[i]) {
			t.Errorf("command %d = %v, want %v", i, gotCmds[i], wantCmds[i])
		}
	}
}

func TestSubmoduleRemoveContinuesPastFailures(t *testing.T) {
	mock := executil.NewMockExecutor()
	mock.RunErr = fmt.Errorf("boom") // every git command fails
	g := NewWithExecutor(mock)
	err := g.SubmoduleRemove("sources/org.example.App")
	if err == nil {
		t.Fatal("expected joined error when steps fail")
	}
	// Even though step 1 fails, it must still attempt deinit, rm, and rev-parse.
	if len(mock.Commands) != 3 {
		t.Errorf("expected 3 git commands attempted, got %d", len(mock.Commands))
	}
}

func TestSubmoduleUpdateInitNonRecursive(t *testing.T) {
	mock := executil.NewMockExecutor()
	g := NewWithExecutor(mock)
	if err := g.SubmoduleUpdateInit(false); err != nil {
		t.Fatalf("SubmoduleUpdateInit: %v", err)
	}
	want := []string{"-c", "safe.directory=*", "submodule", "update", "--init"}
	if !equal(mock.Commands[0].Args, want) {
		t.Errorf("got %v, want %v", mock.Commands[0].Args, want)
	}
}

func TestDiffNameOnlySplitsLines(t *testing.T) {
	mock := executil.NewMockExecutor()
	mock.OutMap["git"] = []byte("a.yaml\nb/c.json\n\n")
	g := NewWithExecutor(mock)
	got, err := g.DiffNameOnly("base", "HEAD")
	if err != nil {
		t.Fatalf("DiffNameOnly: %v", err)
	}
	want := []string{"a.yaml", "b/c.json"}
	if !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSubmoduleRemoveRelativeCommonDir(t *testing.T) {
	mock := executil.NewMockExecutor()
	topDir := t.TempDir()

	mock.OnCommand = func(cmd *executil.MockCommand) {
		if cmd.Name == "git" {
			for _, arg := range cmd.Args {
				if arg == "--git-common-dir" {
					cmd.OutData = []byte(".git\n")
					return
				}
				if arg == "--show-toplevel" {
					cmd.OutData = []byte(topDir + "\n")
					return
				}
			}
		}
	}

	submoduleDir := filepath.Join(topDir, ".git", "modules", "sources/org.example.App")
	if err := os.MkdirAll(submoduleDir, 0755); err != nil {
		t.Fatalf("failed to create mock submodule dir: %v", err)
	}

	g := NewWithExecutor(mock)
	if err := g.SubmoduleRemove("sources/org.example.App"); err != nil {
		t.Fatalf("SubmoduleRemove failed: %v", err)
	}

	// Verify that the submodule dir was actually deleted (meaning path resolved correctly)
	if _, err := os.Stat(submoduleDir); !os.IsNotExist(err) {
		t.Errorf("expected submodule directory %q to be deleted, but it still exists", submoduleDir)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
