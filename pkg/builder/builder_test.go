package builder

import (
	"testing"
)

func TestResolveLinterCmd(t *testing.T) {
	cmd, args := resolveLinterCmd()
	if cmd == "" {
		t.Errorf("expected non-empty command name")
	}
	if cmd == "flatpak" {
		if len(args) < 3 {
			t.Errorf("expected at least 3 arguments for flatpak execution, got %v", args)
		}
		if args[0] != "run" || args[1] != "--command=flatpak-builder-lint" || args[2] != "org.flatpak.Builder" {
			t.Errorf("unexpected flatpak runner args: %v", args)
		}
	} else if cmd != "flatpak-builder-lint" {
		t.Errorf("unexpected command: %s", cmd)
	}
}
