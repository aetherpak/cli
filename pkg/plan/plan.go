package plan

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/gitutil"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"gopkg.in/yaml.v3"
)

const zeroSHA = "0000000000000000000000000000000000000000"

// defaultGit is the git client used by the plan change-detection helpers.
var defaultGit gitutil.Git = gitutil.New()

var runnerByArch = map[string]string{
	"x86_64":  "ubuntu-latest",
	"aarch64": "ubuntu-24.04-arm",
}

// MatrixRow represents a row in the expanded build/import matrix.
type MatrixRow struct {
	Source         string `json:"source"`
	AppID          string `json:"app-id"`
	Manifest       string `json:"manifest,omitempty"`
	Runtime        string `json:"runtime,omitempty"`
	RuntimeVersion string `json:"runtime-version,omitempty"`
	Branch         string `json:"branch"`
	Arch           string `json:"arch"`
	Runner         string `json:"runner"`
	RunLinter      bool   `json:"run-linter,omitempty"`
	BundleURL      string `json:"bundle-url,omitempty"`
	BundleSHA256   string `json:"bundle-sha256,omitempty"`
}

// PlanResult holds the planned execution details.
type PlanResult struct {
	Apps           []string    `json:"apps"`
	Matrix         []MatrixRow `json:"matrix"`
	MatrixManifest []MatrixRow `json:"matrix-manifest"`
	MatrixBundle   []MatrixRow `json:"matrix-bundle"`
	Count          int         `json:"count"`
	CountManifest  int         `json:"count-manifest"`
	CountBundle    int         `json:"count-bundle"`
}

// ComputePlan generates the build matrix and selected app IDs based on git diffs.
func ComputePlan(cfg *config.Config, configPath string, baseSHA string, force string, workflowPath string) (*PlanResult, error) {
	if cfg != nil {
		cfg.Normalize()
	}
	baseSHA = strings.TrimSpace(baseSHA)
	force = strings.TrimSpace(force)
	workflowPath = strings.TrimSpace(workflowPath)

	allIDs := make([]string, len(cfg.Apps))
	for i, app := range cfg.Apps {
		allIDs[i] = app.ID
	}

	var selected []string

	// Check if force is "all" or specific app ID
	if force == "all" {
		logger.Debug("Plan force=all, selecting all apps.")
		selected = allIDs
	} else if force != "" {
		found := false
		for _, id := range allIDs {
			if id == force {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("requested app %q is not in the configuration", force)
		}
		logger.Debug("Plan force=%s, selecting single app.", force)
		selected = []string{force}
	} else {
		// Determine base commit configuration
		var prevApps []config.App
		var hasPrev bool
		if baseSHA != "" && baseSHA != zeroSHA {
			prevData, err := gitShow(baseSHA, configPath)
			if err == nil {
				var prevCfg config.Config
				if err := yaml.Unmarshal(prevData, &prevCfg); err == nil {
					prevCfg.Normalize()
					prevApps = prevCfg.Apps
					hasPrev = true
					logger.Debug("Loaded base SHA config details.")
				}
			}
		}

		// Collect changed files from git
		changedFiles, err := getChangedFiles(baseSHA)
		if err != nil {
			logger.Debug("Failed to get git diff (assuming all changed): %v", err)
			selected = allIDs
		} else if changedFiles == nil {
			logger.Debug("Base SHA not found or unreachable, building all.")
			selected = allIDs
		} else {
			logger.Debug("Changed files since base SHA: %v", changedFiles)

			// If workflow file was changed, rebuild everything
			workflowChanged := false
			if workflowPath != "" {
				for _, cf := range changedFiles {
					if cf == workflowPath {
						workflowChanged = true
						break
					}
				}
			}

			if workflowChanged {
				logger.Debug("Workflow path %s changed, force building all.", workflowPath)
				selected = allIDs
			} else {
				// Select apps based on manifest changes or config differences
				prevMap := make(map[string]config.App)
				for _, app := range prevApps {
					prevMap[app.ID] = app
				}

				selectedSet := make(map[string]bool)
				for _, app := range cfg.Apps {
					// 1. Check if manifest directory was touched
					if app.Manifest != "" {
						manifestDir := filepath.Dir(app.Manifest)
						if manifestDir != "." && manifestDir != "" {
							prefix := manifestDir + "/"
							for _, cf := range changedFiles {
								if cf == manifestDir || strings.HasPrefix(cf, prefix) {
									selectedSet[app.ID] = true
									logger.Debug("App %s manifest directory %s was touched.", app.ID, manifestDir)
									break
								}
							}
						}
					}

					// 2. Check if configuration changed
					if !selectedSet[app.ID] {
						prevApp, exists := prevMap[app.ID]
						if !hasPrev || !exists || !appConfigsEqual(app, prevApp) {
							selectedSet[app.ID] = true
							logger.Debug("App %s config is new or changed.", app.ID)
						}
					}
				}

				for _, id := range allIDs {
					if selectedSet[id] {
						selected = append(selected, id)
					}
				}
			}
		}
	}

	// Expand the matrix
	var rows []MatrixRow
	var manifestRows []MatrixRow
	var bundleRows []MatrixRow

	appMap := make(map[string]config.App)
	for _, app := range cfg.Apps {
		appMap[app.ID] = app
	}

	for _, id := range selected {
		app := appMap[id]
		branch := app.Branch
		if branch == "" {
			branch = "stable"
		}

		if app.Manifest != "" {
			arches := app.Arches
			if len(arches) == 0 {
				arches = []string{"x86_64"}
			}
			for _, arch := range arches {
				row := MatrixRow{
					Source:         "manifest",
					AppID:          id,
					Manifest:       app.Manifest,
					Runtime:        app.Runtime,
					RuntimeVersion: app.RuntimeVersion,
					Branch:         branch,
					Arch:           arch,
					Runner:         runnerByArch[arch],
					RunLinter:      app.RunLinter,
				}
				rows = append(rows, row)
				manifestRows = append(manifestRows, row)
			}
		} else {
			for arch, b := range app.Bundles {
				row := MatrixRow{
					Source:       "bundle",
					AppID:        id,
					Branch:       branch,
					Arch:         arch,
					Runner:       runnerByArch[arch],
					BundleURL:    b.URL,
					BundleSHA256: b.SHA256,
				}
				rows = append(rows, row)
				bundleRows = append(bundleRows, row)
			}
		}
	}

	return &PlanResult{
		Apps:           selected,
		Matrix:         rows,
		MatrixManifest: manifestRows,
		MatrixBundle:   bundleRows,
		Count:          len(selected),
		CountManifest:  len(manifestRows),
		CountBundle:    len(bundleRows),
	}, nil
}

func gitShow(commit, file string) ([]byte, error) {
	if relFile, err := getRepoRelativePath(file); err == nil {
		file = relFile
	}
	return defaultGit.Show(commit, file)
}

func getRepoRelativePath(path string) (string, error) {
	repoRoot, err := defaultGit.Toplevel()
	if err != nil {
		return path, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path, err
	}
	relPath, err := filepath.Rel(repoRoot, absPath)
	if err != nil {
		return path, err
	}
	return relPath, nil
}

func getChangedFiles(baseSHA string) ([]string, error) {
	if baseSHA == "" || baseSHA == zeroSHA {
		return nil, nil
	}
	if !defaultGit.CatFileExists(baseSHA) {
		return nil, nil // base commit unreachable
	}
	return defaultGit.DiffNameOnly(baseSHA, "HEAD")
}

// appConfigsEqual compares two App configs structurally to detect differences.
func appConfigsEqual(a, b config.App) bool {
	return a.Equal(b)
}
