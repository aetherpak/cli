package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/executil"
)

func TestStatusCmdDependencies(t *testing.T) {
	// 1. Success case: All required dependencies are present
	mockExecSuccess := executil.NewMockExecutor()
	mockExecSuccess.PathMap["flatpak"] = "/usr/bin/flatpak"
	mockExecSuccess.PathMap["flatpak-builder"] = "/usr/bin/flatpak-builder"
	mockExecSuccess.PathMap["ostree"] = "/usr/bin/ostree"

	oldExecutor := statusExecutor
	defer func() { statusExecutor = oldExecutor }()

	statusExecutor = mockExecSuccess
	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)

	err := statusCmd.RunE(statusCmd, nil)
	if err != nil {
		t.Fatalf("expected status command to succeed when required dependencies are present, got: %v", err)
	}

	// 2. Failure case: A required dependency (flatpak-builder) is missing
	mockExecFailure := executil.NewMockExecutor()
	mockExecFailure.PathMap["flatpak"] = "/usr/bin/flatpak"
	mockExecFailure.PathMap["ostree"] = "/usr/bin/ostree"
	// flatpak-builder is missing from PathMap

	statusExecutor = mockExecFailure
	err = statusCmd.RunE(statusCmd, nil)
	if err == nil {
		t.Fatal("expected status command to fail when required dependency is missing, got nil")
	}
	if !strings.Contains(err.Error(), "missing required system dependency") {
		t.Errorf("expected missing required system dependency error, got: %v", err)
	}
}
