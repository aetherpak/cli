package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRenderPremiumHelp(t *testing.T) {
	// Create a mock command tree
	root := &cobra.Command{
		Use:   "root",
		Short: "Root command short desc",
		Long:  "Root command long desc",
	}
	sub := &cobra.Command{
		Use:   "sub",
		Short: "Sub command short desc",
	}
	root.AddCommand(sub)

	// Add a flag
	var flagVal string
	root.PersistentFlags().StringVar(&flagVal, "testflag", "", "A flag for testing")

	var buf bytes.Buffer
	renderPremiumHelp(root, &buf)

	output := buf.String()
	if !strings.Contains(output, "USAGE") {
		t.Error("expected output to contain USAGE section")
	}
	if !strings.Contains(output, "AVAILABLE COMMANDS") {
		t.Error("expected output to contain AVAILABLE COMMANDS section")
	}
	if !strings.Contains(output, "FLAGS") {
		t.Error("expected output to contain FLAGS section")
	}
	if !strings.Contains(output, "sub") {
		t.Error("expected output to contain sub command")
	}
	if !strings.Contains(output, "testflag") {
		t.Error("expected output to contain testflag")
	}
}
