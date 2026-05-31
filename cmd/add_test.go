package cmd

import (
	"testing"

	"github.com/aetherpak/aetherpak/pkg/adder"
)

func TestAddFlagsRegistered(t *testing.T) {
	c, _, err := RootCmd.Find([]string{"add"})
	if err != nil {
		t.Fatalf("find add: %v", err)
	}
	flags := []string{"manifest", "bundle-url", "git", "git-manifest", "submodule-path", "id", "branch", "arch", "bundle-sha256", "confirm", "builder-arg"}
	// Every registry option must be exposed as a flag (DRY guarantee).
	for _, opt := range adder.BoolOptions {
		flags = append(flags, opt.Key)
	}
	for _, f := range flags {
		if c.Flags().Lookup(f) == nil {
			t.Errorf("add: missing --%s flag", f)
		}
	}
}
