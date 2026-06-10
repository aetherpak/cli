package cmd

import (
	"github.com/aetherpak/aetherpak/pkg/scm"
	"github.com/aetherpak/aetherpak/pkg/site"
	"github.com/spf13/cobra"
)

var (
	previewTemplatePath    string
	previewLive            bool
	previewLiveURL         string
	previewGPG             bool
	previewApps            string
	previewSiteDir         string
	previewNoServe         bool
	previewPort            int
	previewRemoteName      string
	previewRepoTitle       string
	previewRepoHP          string
	previewDefaultTemplate bool
)

var previewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Generates and serves a local preview of the landing page template",
	Long: `Generates a static site structure in the preview directory using either dummy or live data
and optionally launches a local HTTP server. This allows testing layout and design changes.

To generate files without starting the server, use: --serve=false`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}

		if previewRemoteName == "" {
			previewRemoteName = cfg.RemoteName
		}
		if previewRemoteName == "" {
			previewRemoteName = scm.RemoteName()
		}
		if previewRepoTitle == "" {
			previewRepoTitle = cfg.RepoTitle
		}
		if previewRepoHP == "" {
			previewRepoHP = cfg.RepoHomepage
		}

		pagesURL := cfg.PagesURL
		if pagesURL == "" {
			pagesURL = scm.PagesURL()
		}

		if previewTemplatePath == "" && !previewDefaultTemplate && cfg.Branding != nil {
			previewTemplatePath = cfg.Branding.IndexTemplate
		}

		opts := site.PreviewOptions{
			TemplatePath: previewTemplatePath,
			SiteDir:      previewSiteDir,
			Live:         previewLive || previewLiveURL != "",
			LiveURL:      previewLiveURL,
			GPG:          previewGPG,
			Apps:         previewApps,
			PagesURL:     pagesURL,
			RemoteName:   previewRemoteName,
			RepoTitle:    previewRepoTitle,
			RepoHomepage: previewRepoHP,
		}

		if err := site.GeneratePreview(opts); err != nil {
			return NewCmdError(1, err)
		}

		if !previewNoServe {
			if err := site.StartPreviewServer(previewSiteDir, previewPort); err != nil {
				return NewCmdError(1, err)
			}
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(previewCmd)

	previewCmd.Flags().StringVarP(&previewTemplatePath, "template", "t", "", "path to custom HTML repository index template")
	previewCmd.Flags().BoolVar(&previewLive, "live", false, "fetch live production index data instead of generating dummy data")
	previewCmd.Flags().StringVar(&previewLiveURL, "live-url", "", "live pages URL to download the index from (implicitly enables --live)")
	previewCmd.Flags().BoolVar(&previewGPG, "gpg", true, "simulate GPG signing for dummy data")
	previewCmd.Flags().StringVar(&previewApps, "apps", "multiple", "simulate 'single' or 'multiple' applications in dummy data")
	previewCmd.Flags().StringVar(&previewSiteDir, "site-dir", "_preview", "destination directory for preview assets")
	previewCmd.Flags().BoolVar(&previewNoServe, "no-serve", false, "do not start a local HTTP server to preview the site")
	previewCmd.Flags().IntVar(&previewPort, "port", 8080, "port for local HTTP server")
	previewCmd.Flags().StringVar(&previewRemoteName, "remote-name", "", "flatpak remote name for generated references")
	previewCmd.Flags().StringVar(&previewRepoTitle, "repo-title", "", "title for the generated .flatpakrepo file")
	previewCmd.Flags().StringVar(&previewRepoHP, "repo-homepage", "", "homepage URL for the generated .flatpakrepo file")
	previewCmd.Flags().BoolVar(&previewDefaultTemplate, "default-template", false, "force using the default landing page template, ignoring configuration/env settings")
}
