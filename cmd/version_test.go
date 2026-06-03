package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	// Keep original version state to restore after test
	oldVersion := Version
	defer func() {
		Version = oldVersion
		RootCmd.Version = oldVersion
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
	}()

	Version = "v0.12.3-test"
	RootCmd.Version = Version

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetArgs([]string{"--version"})

	err := RootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error executing command: %v", err)
	}

	output := buf.String()
	expected := "aetherpak version v0.12.3-test"
	if !strings.Contains(output, expected) {
		t.Errorf("expected output to contain %q, got %q", expected, output)
	}
}
