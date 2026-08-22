package appstream

import (
	"os"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
)

func TestSyncOSTreeCommitAppStream(t *testing.T) {
	initialXML := `<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>org.example.App</id>
  <name>Example App</name>
  <releases>
    <release version="1.0.0" date="2025-01-01"/>
  </releases>
</component>`

	var commandsExecuted []string

	mockExec := executil.NewMockExecutor()
	mockExec.OnCommand = func(cmd *executil.MockCommand) {
		fullCmd := cmd.Name + " " + strings.Join(cmd.Args, " ")
		commandsExecuted = append(commandsExecuted, fullCmd)

		if cmd.Name == "ostree" && len(cmd.Args) >= 3 && cmd.Args[0] == "ls" {
			cmd.OutData = []byte("-rw-r--r-- 0 0 1000 /files/share/metainfo/org.example.App.metainfo.xml\n")
		}
		if cmd.Name == "ostree" && len(cmd.Args) >= 3 && cmd.Args[0] == "show" {
			cmd.OutData = []byte("xa.metadata\nxa.ref\n")
		}
		if cmd.Name == "ostree" && len(cmd.Args) >= 3 && cmd.Args[0] == "cat" {
			cmd.OutData = []byte(initialXML)
		}
	}

	opts := ReleaseOptions{
		Version: "v1.2.0",
		Date:    "2026-08-22",
	}

	modified, err := SyncOSTreeCommitAppStream(mockExec, "repo", "app/org.example.App/x86_64/master", opts)
	if err != nil {
		t.Fatalf("SyncOSTreeCommitAppStream failed: %v", err)
	}
	if !modified {
		t.Fatalf("expected modified=true")
	}

	// Verify command sequence
	hasCheckout := false
	hasCommit := false
	hasUpdateRepo := false
	hasKeepMetadata := false
	for _, cmd := range commandsExecuted {
		if strings.Contains(cmd, "ostree checkout") {
			hasCheckout = true
		}
		if strings.Contains(cmd, "ostree commit") {
			hasCommit = true
			if strings.Contains(cmd, "--keep-metadata=xa.metadata") && strings.Contains(cmd, "--parent=") {
				hasKeepMetadata = true
			}
		}
		if strings.Contains(cmd, "flatpak build-update-repo") {
			hasUpdateRepo = true
		}
	}

	if !hasCheckout {
		t.Errorf("expected ostree checkout command in execution")
	}
	if !hasCommit {
		t.Errorf("expected ostree commit command in execution")
	}
	if !hasKeepMetadata {
		t.Errorf("expected ostree commit to preserve metadata keys via --keep-metadata")
	}
	if !hasUpdateRepo {
		t.Errorf("expected flatpak build-update-repo command in execution")
	}
}

func TestSyncOSTreeCommitAppStream_SymlinkTarget(t *testing.T) {
	initialXML := `<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>org.example.App</id>
  <releases>
    <release version="1.0.0" date="2025-01-01"/>
  </releases>
</component>`

	mockExec := executil.NewMockExecutor()
	mockExec.OnCommand = func(cmd *executil.MockCommand) {
		if cmd.Name == "ostree" && len(cmd.Args) >= 3 && cmd.Args[0] == "ls" {
			cmd.OutData = []byte("-rw-r--r-- 0 0 1000 /files/share/metainfo/org.example.App.metainfo.xml\n")
		}
		if cmd.Name == "ostree" && len(cmd.Args) >= 3 && cmd.Args[0] == "cat" {
			cmd.OutData = []byte(initialXML)
		}
		if cmd.Name == "ostree" && len(cmd.Args) >= 3 && cmd.Args[0] == "checkout" {
			// Create checkout dir with a symlinked metainfo file pointing outside checkout
			checkoutDir := cmd.Args[len(cmd.Args)-1]
			metaDir := checkoutDir + "/files/share/metainfo"
			_ = os.MkdirAll(metaDir, 0755)
			_ = os.Symlink("/etc/passwd", metaDir+"/org.example.App.metainfo.xml")
		}
	}

	opts := ReleaseOptions{
		Version: "v1.2.0",
		Date:    "2026-08-22",
	}

	_, err := SyncOSTreeCommitAppStream(mockExec, "repo", "app/org.example.App/x86_64/master", opts)
	if err == nil {
		t.Fatalf("expected error when metainfo target is a symlink, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected error mentioning symlink, got: %v", err)
	}
}
