package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/snippets"
	"github.com/spf13/cobra"
)

var (
	snippetFormat     string
	snippetAppID      string
	snippetChannel    string
	snippetPagesURL   string
	snippetRemoteName string
	snippetNoSign     bool
	snippetOutputFile string
)

var snippetsCmd = &cobra.Command{
	Use:     "snippets",
	Aliases: []string{"snippet"},
	Short:   "Generate ready-to-use installation snippets in Markdown, HTML, or JSON",
	Long: `Generates ready-to-use installation and channel-switching documentation snippets
for Stable, Nightly, and Flatpakref one-click installs based on your repository configuration.

Supported formats:
  markdown (default, alias: md) - GitHub Flavored Markdown
  html                          - Semantic HTML elements
  json                          - Structured JSON data for custom site generators`,
	Example: `  # Generate default Markdown snippets
  aetherpak snippets

  # Generate HTML snippets for a specific application
  aetherpak snippets --format=html --app=org.example.App

  # Generate JSON metadata for static site generators
  aetherpak snippets --format=json

  # Save snippets directly to a file
  aetherpak snippets --format=markdown --output-file=INSTALL.md`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}

		format := strings.ToLower(strings.TrimSpace(snippetFormat))
		if format == "" {
			format = "markdown"
		}

		switch format {
		case "markdown", "md", "html", "json":
			// valid format
		default:
			return NewCmdErrorf(1, "unsupported format %q (must be 'markdown', 'md', 'html', or 'json')", snippetFormat)
		}

		var noSignPtr *bool
		if cmd.Flags().Changed("no-sign") {
			val := snippetNoSign
			noSignPtr = &val
		}

		opts := snippets.SnippetOptions{
			Format:     format,
			AppID:      snippetAppID,
			Channel:    snippetChannel,
			RemoteName: snippetRemoteName,
			PagesURL:   snippetPagesURL,
			NoSign:     noSignPtr,
		}

		res, err := snippets.GenerateSnippets(cfg, opts)
		if err != nil {
			return NewCmdError(1, err)
		}

		var output string
		switch format {
		case "markdown", "md":
			output, err = snippets.FormatMarkdown(res)
		case "html":
			output, err = snippets.FormatHTML(res)
		case "json":
			output, err = snippets.FormatJSON(res)
		}

		if err != nil {
			return NewCmdError(1, err)
		}

		if snippetOutputFile != "" && snippetOutputFile != "-" {
			if err := os.WriteFile(snippetOutputFile, []byte(output), 0644); err != nil {
				return NewCmdErrorf(1, "failed to write output file %s: %w", snippetOutputFile, err)
			}
			logger.SuccessBanner("Snippets Generated", fmt.Sprintf("Successfully written install snippets to: %s", snippetOutputFile))
		} else {
			fmt.Fprint(cmd.OutOrStdout(), output)
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(snippetsCmd)

	snippetsCmd.Flags().StringVarP(&snippetFormat, "format", "f", "markdown", "output format (markdown, html, json)")
	snippetsCmd.Flags().StringVarP(&snippetAppID, "app", "a", "", "filter snippets by application ID")
	snippetsCmd.Flags().StringVarP(&snippetChannel, "channel", "c", "", "filter snippets by channel/branch (e.g. stable, nightly)")
	snippetsCmd.Flags().StringVar(&snippetPagesURL, "pages-url", "", "override base Pages repository URL")
	snippetsCmd.Flags().StringVar(&snippetRemoteName, "remote-name", "", "override Flatpak remote repository name")
	snippetsCmd.Flags().BoolVar(&snippetNoSign, "no-sign", false, "generate unverified/unsigned repository commands")
	snippetsCmd.Flags().StringVarP(&snippetOutputFile, "output-file", "o", "", "write generated snippets to specified file path (- or empty for stdout)")
}
