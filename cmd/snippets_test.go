package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/snippets"
	"github.com/spf13/viper"
)

func TestSnippetsCmd_Default(t *testing.T) {
	viper.Reset()
	snippetFormat = "markdown"
	snippetAppID = ""
	snippetChannel = ""
	snippetPagesURL = ""
	snippetRemoteName = ""
	snippetNoSign = false
	snippetOutputFile = ""

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetArgs([]string{"snippets", "--remote-name=testrepo", "--pages-url=https://test.github.io/repo"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("unexpected command error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Flatpak Installation Guide") {
		t.Errorf("expected installation guide header, got:\n%s", out)
	}
	if !strings.Contains(out, "flatpak remote-add --if-not-exists --user testrepo https://test.github.io/repo/testrepo.flatpakrepo") {
		t.Errorf("expected remote add command, got:\n%s", out)
	}
	if !strings.Contains(out, "org.example.App") {
		t.Errorf("expected default app ID org.example.App, got:\n%s", out)
	}
}

func TestSnippetsCmd_FormatMDAlias(t *testing.T) {
	viper.Reset()
	snippetFormat = "md"
	snippetAppID = ""
	snippetChannel = ""
	snippetPagesURL = ""
	snippetRemoteName = ""
	snippetNoSign = false
	snippetOutputFile = ""

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetArgs([]string{"snippets", "--format=md", "--remote-name=testrepo", "--pages-url=https://test.github.io/repo"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("unexpected command error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Flatpak Installation Guide") {
		t.Errorf("expected installation guide header, got:\n%s", out)
	}
}

func TestSnippetsCmd_FormatHTML(t *testing.T) {
	viper.Reset()
	snippetFormat = "html"
	snippetAppID = ""
	snippetChannel = ""
	snippetPagesURL = ""
	snippetRemoteName = ""
	snippetNoSign = false
	snippetOutputFile = ""

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetArgs([]string{"snippets", "--format=html", "--remote-name=myrepo", "--pages-url=https://myrepo.org"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("unexpected command error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "<div class=\"aetherpak-snippets\">") {
		t.Errorf("expected HTML root tag, got:\n%s", out)
	}
	if !strings.Contains(out, "btn-flatpakref") {
		t.Errorf("expected btn-flatpakref class in HTML, got:\n%s", out)
	}
}

func TestSnippetsCmd_FormatJSON(t *testing.T) {
	viper.Reset()
	snippetFormat = "json"
	snippetAppID = ""
	snippetChannel = ""
	snippetPagesURL = ""
	snippetRemoteName = ""
	snippetNoSign = false
	snippetOutputFile = ""

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetArgs([]string{"snippets", "--format=json", "--remote-name=myrepo", "--pages-url=https://myrepo.org"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("unexpected command error: %v", err)
	}

	var res snippets.SnippetResult
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v\nOutput was:\n%s", err, buf.String())
	}

	if res.Repo.RemoteName != "myrepo" {
		t.Errorf("expected remote name myrepo, got %s", res.Repo.RemoteName)
	}
	if len(res.Apps) == 0 {
		t.Errorf("expected apps in result, got 0")
	}
}

func TestSnippetsCmd_OutputFile(t *testing.T) {
	viper.Reset()
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "install-guide.md")

	snippetFormat = "markdown"
	snippetAppID = ""
	snippetChannel = ""
	snippetPagesURL = ""
	snippetRemoteName = ""
	snippetNoSign = false
	snippetOutputFile = outFile

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetArgs([]string{"snippets", "--output-file=" + outFile, "--remote-name=testrepo", "--pages-url=https://test.io"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("unexpected command error: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read written output file: %v", err)
	}

	if !strings.Contains(string(data), "Flatpak Installation Guide") {
		t.Errorf("expected file content to contain guide header, got:\n%s", string(data))
	}
}

func TestSnippetsCmd_InvalidFormat(t *testing.T) {
	viper.Reset()
	snippetFormat = "yaml"
	snippetAppID = ""
	snippetChannel = ""
	snippetPagesURL = ""
	snippetRemoteName = ""
	snippetNoSign = false
	snippetOutputFile = ""

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetArgs([]string{"snippets", "--format=yaml"})

	err := RootCmd.Execute()
	if err == nil {
		t.Fatalf("expected error for invalid format yaml, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("expected unsupported format error message, got: %v", err)
	}
}
