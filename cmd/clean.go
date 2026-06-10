package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var (
	cleanYes     bool
	cleanCCache  bool
	cleanState   bool
	cleanPreview bool
	cleanSite    bool
	cleanRecords bool
	cleanRepo    bool

	cleanCCacheDir  string
	cleanStateDir   string
	cleanPreviewDir string
	cleanSiteDir    string
	cleanRecordsDir string
	cleanRepoPath   string
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clears builder caches, temporary preview files, and build state",
	Long: `Clears builder compiler caches, state directories, template preview files,
production site outputs, build-site records, and local OSTree repositories.
Requires confirmation unless --yes or -y is specified.`,
	RunE: runClean,
}

func runClean(cmd *cobra.Command, args []string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return NewCmdErrorf(2, "Configuration error: %w", err)
	}

	// Determine what targets to clean. If no specific target filters are set, we clean all.
	all := !cmd.Flags().Changed("ccache") &&
		!cmd.Flags().Changed("state") &&
		!cmd.Flags().Changed("preview") &&
		!cmd.Flags().Changed("site") &&
		!cmd.Flags().Changed("records") &&
		!cmd.Flags().Changed("repo")

	cCCache := all || cleanCCache
	cState := all || cleanState
	cPreview := all || cleanPreview
	cSite := all || cleanSite
	cRecords := all || cleanRecords
	cRepo := all || cleanRepo

	dirsToClean := make(map[string]string)

	addDir := func(path string, label string) {
		if path == "" {
			return
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			absPath = path
		}
		if fi, err := os.Stat(absPath); err == nil && fi.IsDir() {
			dirsToClean[absPath] = label
		}
	}

	// 1. Resolve Compiler Cache (ccache) Directories
	if cCCache {
		// Use CLI flag if changed, otherwise use configuration
		if cmd.Flags().Changed("ccache-dir") {
			addDir(cleanCCacheDir, "compiler cache (ccache)")
		} else {
			globalCCache := cfg.Defaults.CCacheDir
			if globalCCache == "" {
				if cfg.OutputDir != "" {
					globalCCache = filepath.Join(cfg.OutputDir, ".ccache")
				} else {
					globalCCache = ".ccache"
				}
			}
			addDir(globalCCache, "compiler cache (ccache)")

			for _, app := range cfg.Apps {
				if app.CCacheDir != "" {
					addDir(app.CCacheDir, fmt.Sprintf("compiler cache (ccache) for app %s", app.ID))
				}
			}
		}
	}

	// 2. Resolve Builder State Directories
	if cState {
		// Use CLI flag if changed, otherwise use configuration
		if cmd.Flags().Changed("state-dir") {
			addDir(cleanStateDir, "builder state")
		} else {
			globalState := cfg.Defaults.StateDir
			if globalState == "" {
				if cfg.OutputDir != "" {
					globalState = filepath.Join(cfg.OutputDir, ".state")
				} else {
					globalState = ".state"
				}
			}
			addDir(globalState, "builder state")

			for _, app := range cfg.Apps {
				if app.StateDir != "" {
					addDir(app.StateDir, fmt.Sprintf("builder state for app %s", app.ID))
				}
			}
		}
	}

	// 3. Resolve Template Preview Directory
	if cPreview {
		previewDir := cleanPreviewDir
		if !cmd.Flags().Changed("preview-dir") && cfg.OutputDir != "" {
			// Check if output_dir is configured; if so, we also check if preview was built there
			// but normally preview command is relative to workdir unless specified.
			previewDir = filepath.Join(cfg.OutputDir, "_preview")
			addDir(previewDir, "template preview files")
		}
		// Always check default preview-dir
		addDir(cleanPreviewDir, "template preview files")
	}

	// 4. Resolve Production Site Outputs
	if cSite {
		siteDirVal := cleanSiteDir
		if !cmd.Flags().Changed("site-dir") && cfg.OutputDir != "" {
			siteDirVal = filepath.Join(cfg.OutputDir, "_site")
		}
		addDir(siteDirVal, "production site outputs")
	}

	// 5. Resolve Build-Site Records
	if cRecords {
		recordsDirVal := cleanRecordsDir
		if !cmd.Flags().Changed("records-dir") && cfg.OutputDir != "" {
			recordsDirVal = filepath.Join(cfg.OutputDir, "records")
		}
		addDir(recordsDirVal, "build-site records")
	}

	// 6. Resolve Local OSTree Repositories
	if cRepo {
		repoPathVal := cleanRepoPath
		if !cmd.Flags().Changed("repo-path") && cfg.OutputDir != "" {
			repoPathVal = filepath.Join(cfg.OutputDir, "repo")
		}
		addDir(repoPathVal, "local OSTree repository")
	}

	if len(dirsToClean) == 0 {
		logger.Info("Nothing to clean.")
		return nil
	}

	// Confirm unless -y / --yes / --confirm is specified
	if !cleanYes {
		if isInteractive() {
			var appDetails []string
			var paths []string
			for p := range dirsToClean {
				paths = append(paths, p)
			}
			sort.Strings(paths)

			for _, p := range paths {
				appDetails = append(appDetails, fmt.Sprintf("%s (%s)", p, dirsToClean[p]))
			}

			var confirm bool
			err := huh.NewConfirm().
				Title(fmt.Sprintf("Do you want to delete the following directories?\n- %s", strings.Join(appDetails, "\n- "))).
				Value(&confirm).
				Run()
			if err != nil {
				return err
			}
			if !confirm {
				logger.Info("Clean cancelled.")
				return nil
			}
		} else {
			return NewCmdErrorf(1, "confirmation required; use --yes or -y to bypass")
		}
	}

	// Sort paths to ensure a deterministic execution order
	var paths []string
	for p := range dirsToClean {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// Execute deletion
	for _, path := range paths {
		label := dirsToClean[path]
		logger.Info("Deleting %s: %s", label, path)
		if err := os.RemoveAll(path); err != nil {
			return NewCmdErrorf(1, "failed to delete %s: %w", path, err)
		}
	}

	logger.SuccessBanner("Clean Completed", "Successfully cleared builder caches and temporary files.")
	return nil
}

func init() {
	RootCmd.AddCommand(cleanCmd)

	cleanCmd.Flags().BoolVarP(&cleanYes, "yes", "y", false, "skip confirmation prompt")
	cleanCmd.Flags().BoolVar(&cleanYes, "confirm", false, "skip confirmation prompt (deprecated)")
	_ = cleanCmd.Flags().MarkDeprecated("confirm", "please use --yes instead")

	cleanCmd.Flags().BoolVar(&cleanCCache, "ccache", false, "clean only builder ccache directories")
	cleanCmd.Flags().BoolVar(&cleanState, "state", false, "clean only builder state directories")
	cleanCmd.Flags().BoolVar(&cleanPreview, "preview", false, "clean only preview directories")
	cleanCmd.Flags().BoolVar(&cleanSite, "site", false, "clean only production site build outputs")
	cleanCmd.Flags().BoolVar(&cleanRecords, "records", false, "clean only build-site records")
	cleanCmd.Flags().BoolVar(&cleanRepo, "repo", false, "clean only local OSTree repositories")

	cleanCmd.Flags().StringVar(&cleanCCacheDir, "ccache-dir", ".ccache", "ccache directory path")
	cleanCmd.Flags().StringVar(&cleanStateDir, "state-dir", ".state", "builder state directory path")
	cleanCmd.Flags().StringVar(&cleanPreviewDir, "preview-dir", "_preview", "preview site directory path")
	cleanCmd.Flags().StringVar(&cleanSiteDir, "site-dir", "_site", "production site directory path")
	cleanCmd.Flags().StringVar(&cleanRecordsDir, "records-dir", "records", "records directory path")
	cleanCmd.Flags().StringVar(&cleanRepoPath, "repo-path", "repo", "OSTree repository path")
}
