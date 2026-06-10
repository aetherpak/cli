package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/scm"
	"github.com/aetherpak/aetherpak/pkg/site"
	"github.com/spf13/cobra"
)

var (
	sitePagesURL      string
	siteRecordsDir    string
	siteDir           string
	siteReconcile     bool
	siteGPGKeys       []string
	siteGPGPassphrase string
	siteRemoteName    string
	siteRuntimeRepo   string
	siteRepoTitle     string
	siteRepoHP        string
	siteLandingPage   bool
	siteInsecure      bool
	siteOutputFile    string
	siteIndexTemplate string
	siteNoSign        bool
	siteAllowUnsigned bool
)

var buildSiteCmd = &cobra.Command{
	Use:   "build-site",
	Short: "Assembles site index and generates deployable assets",
	Long:  `Downloads active static index from hosting pages, merges recent OCI cell records, cleans up missing registry items, and writes flatpakrepo files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}

		if siteRemoteName == "" {
			siteRemoteName = cfg.RemoteName
		}
		if siteRemoteName == "" {
			siteRemoteName = scm.RemoteName()
		}
		if siteRuntimeRepo == "" {
			siteRuntimeRepo = cfg.RuntimeRepo
		}
		if siteRepoTitle == "" {
			siteRepoTitle = cfg.RepoTitle
		}
		if siteRepoHP == "" {
			siteRepoHP = cfg.RepoHomepage
		}

		if sitePagesURL == "" {
			sitePagesURL = cfg.PagesURL
		}
		if sitePagesURL == "" {
			sitePagesURL = scm.PagesURL()
		}

		recordsDir := siteRecordsDir
		if !cmd.Flags().Changed("records-dir") && cfg.OutputDir != "" {
			recordsDir = filepath.Join(cfg.OutputDir, "records")
		} else if recordsDir == "" {
			recordsDir = "records"
		}

		siteDirVal := siteDir
		if !cmd.Flags().Changed("site-dir") && cfg.OutputDir != "" {
			siteDirVal = filepath.Join(cfg.OutputDir, "_site")
		} else if siteDirVal == "" {
			siteDirVal = "_site"
		}

		keys := siteGPGKeys

		var passphrase []byte
		if siteGPGPassphrase != "" {
			passphrase = []byte(siteGPGPassphrase)
		}
		defer func() {
			if len(passphrase) > 0 {
				for i := range passphrase {
					passphrase[i] = 0
				}
			}
		}()

		var brandLogo, brandFavicon, brandAccent, brandFooter, brandTemplate string
		if cfg != nil && cfg.Branding != nil {
			brandLogo = cfg.Branding.LogoURL
			brandFavicon = cfg.Branding.FaviconURL
			brandAccent = cfg.Branding.AccentColor
			brandFooter = cfg.Branding.FooterText
			brandTemplate = cfg.Branding.IndexTemplate
		}

		if siteIndexTemplate == "" {
			siteIndexTemplate = brandTemplate
		}

		var activeAppIDs []string
		if cfg != nil {
			for _, app := range cfg.Apps {
				if app.ID != "" {
					activeAppIDs = append(activeAppIDs, app.ID)
				}
			}
		}

		var activeOCIRepo string
		if cfg != nil {
			activeOCIRepo = cfg.OCIRepository
		}
		if activeOCIRepo == "" {
			activeOCIRepo = scm.OCIRepository()
		}
		if activeOCIRepo == "" && cfg != nil {
			activeOCIRepo = cfg.RemoteName
		}

		opts := site.SiteOptions{
			PagesURL:            sitePagesURL,
			RecordsDir:          recordsDir,
			SiteDir:             siteDirVal,
			Reconcile:           siteReconcile,
			ActiveAppIDs:        activeAppIDs,
			ActiveOCIRepository: activeOCIRepo,
			GPGKeys:             keys,
			GPGPassphrase:       passphrase,
			RemoteName:          siteRemoteName,
			RuntimeRepo:         siteRuntimeRepo,
			RepoTitle:           siteRepoTitle,
			RepoHomepage:        siteRepoHP,
			LandingPage:         siteLandingPage,
			Insecure:            siteInsecure,
			LogoURL:             brandLogo,
			FaviconURL:          brandFavicon,
			AccentColor:         brandAccent,
			FooterText:          brandFooter,
			IndexTemplate:       siteIndexTemplate,
			NoSign:              siteNoSign,
			AllowUnsigned:       siteAllowUnsigned,
		}

		if err := site.BuildSite(opts); err != nil {
			return NewCmdError(1, err)
		}
		if err := ciout.Emit(siteOutputFile, []ciout.KV{
			{Key: "site-dir", Value: siteDirVal},
			{Key: "records-dir", Value: recordsDir},
		}); err != nil {
			return NewCmdError(1, err)
		}
		logger.SuccessBanner("Site Build Completed", fmt.Sprintf("Successfully built static index site at: %s", siteDirVal))
		return nil
	},
}

func init() {
	RootCmd.AddCommand(buildSiteCmd)

	buildSiteCmd.Flags().StringVar(&sitePagesURL, "pages-url", "", "URL of the target Pages server hosting the repository index")
	buildSiteCmd.Flags().StringVar(&siteRecordsDir, "records-dir", "records", "directory containing parallel records")
	buildSiteCmd.Flags().StringVar(&siteDir, "site-dir", "_site", "destination directory for static site assets")
	buildSiteCmd.Flags().BoolVar(&siteReconcile, "reconcile", false, "verify OCI image tags and prune missing index listings")
	buildSiteCmd.Flags().StringSliceVar(&siteGPGKeys, "gpg-key", nil, "GPG private key block(s) or path(s) to private key file(s)")
	buildSiteCmd.Flags().StringVar(&siteRemoteName, "remote-name", "", "flatpak remote name for generated references")
	buildSiteCmd.Flags().StringVar(&siteRuntimeRepo, "runtime-repo", "", "URL for the runtime repository (.flatpakrepo)")
	buildSiteCmd.Flags().StringVar(&siteRepoTitle, "repo-title", "", "title for the generated .flatpakrepo file")
	buildSiteCmd.Flags().StringVar(&siteRepoHP, "repo-homepage", "", "homepage URL for the generated .flatpakrepo file")
	buildSiteCmd.Flags().BoolVar(&siteLandingPage, "landing-page", false, "generate an index.html landing page")
	buildSiteCmd.Flags().BoolVar(&siteInsecure, "insecure", false, "allow HTTP registry when reconciling")
	buildSiteCmd.Flags().StringVar(&siteGPGPassphrase, "gpg-key-passphrase", "", "passphrase unlocking the GPG private key(s)")
	buildSiteCmd.Flags().StringVar(&siteOutputFile, "output-file", "", "write resolved outputs as dotenv KEY=VALUE (- or empty = stdout)")
	buildSiteCmd.Flags().StringVar(&siteIndexTemplate, "index-template", "", "path to custom HTML repository index template")
	buildSiteCmd.Flags().BoolVar(&siteNoSign, "no-sign", false, "disable GPG signing of repositories/images")
	buildSiteCmd.Flags().BoolVar(&siteAllowUnsigned, "allow-unsigned", false, "allow publishing unsigned repository/images")
}
