package cmd

import "testing"

// Guards that every command consumed by an action exposes --output-file.
func TestOutputFileFlagsRegistered(t *testing.T) {
	for _, name := range []string{"build", "import", "push-oci", "plan"} {
		c, _, err := RootCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		if c.Flags().Lookup("output-file") == nil {
			t.Errorf("%s: missing --output-file flag", name)
		}
	}
}
