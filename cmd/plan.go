package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/manifest"
	"github.com/aetherpak/aetherpak/pkg/plan"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	baseSHA            string
	forceFlag          string
	workflowPath       string
	outputFormat       string
	planOutputFile     string
	planManifest       string
	planArches         []string
	planBranch         string
	planOverrideBranch string
	planDisableLinter  bool
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Computes changes and plans flatpak build matrix",
	Long:  `Computes the diff between git refs and expands aetherpak.yaml to generate a matrix of target apps to build.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var cfg *config.Config
		var configPath string
		var localForce string

		if planManifest != "" {
			if IsFlagExplicitlySet(cmd, "force") && forceFlag != "" {
				return NewCmdError(2, fmt.Errorf("cannot use both --manifest and --force flags together"))
			}
			forceFlag = ""

			manifestData, err := manifest.ParseManifest(planManifest)
			if err != nil {
				return NewCmdErrorf(2, "Manifest parsing error: %w", err)
			}

			// Default target architectures if empty
			if len(planArches) == 0 {
				planArches = []string{"x86_64"}
			}

			if planBranch == "" {
				planBranch = "stable"
			}

			// Construct synthetic config
			syntheticApp := config.App{
				ID:             manifestData.ID,
				Manifest:       planManifest,
				Runtime:        manifestData.Runtime,
				RuntimeVersion: manifestData.RuntimeVersion,
				Arches:         planArches,
				Branch:         planBranch,
				RunLinter:      !planDisableLinter,
			}

			if err := syntheticApp.ValidateBasic(); err != nil {
				return NewCmdErrorf(2, "Manifest validation error: %w", err)
			}

			cfg = &config.Config{
				Apps: []config.App{syntheticApp},
			}
			localForce = ""
		} else {
			var err error
			cfg, err = LoadConfig()
			if err != nil {
				return NewCmdErrorf(2, "Configuration error: %w", err)
			}

			configPath = viper.ConfigFileUsed()
			if configPath == "" {
				if vCfgFile := viper.GetString("config"); vCfgFile != "" {
					configPath = vCfgFile
				} else {
					configPath = "aetherpak.yaml"
					if _, err := os.Stat("aetherpak.yml"); err == nil {
						configPath = "aetherpak.yml"
					}
				}
			}
			localForce = forceFlag
		}

		var forceBranch string
		if planManifest == "" && localForce != "" && localForce != "all" {
			cleanForce, br := parseAppIDRef(localForce)
			localForce = cleanForce
			forceBranch = br
		}

		res, err := plan.ComputePlan(cfg, configPath, baseSHA, localForce, workflowPath)
		if err != nil {
			return NewCmdErrorf(1, "Plan computation error: %w", err)
		}

		if planOverrideBranch != "" || forceBranch != "" {
			for i := range res.Matrix {
				if planOverrideBranch != "" {
					res.Matrix[i].Branch = planOverrideBranch
				}
				if forceBranch != "" && res.Matrix[i].AppID == localForce {
					res.Matrix[i].Branch = forceBranch
				}
			}
			for i := range res.MatrixManifest {
				if planOverrideBranch != "" {
					res.MatrixManifest[i].Branch = planOverrideBranch
				}
				if forceBranch != "" && res.MatrixManifest[i].AppID == localForce {
					res.MatrixManifest[i].Branch = forceBranch
				}
			}
			for i := range res.MatrixBundle {
				if planOverrideBranch != "" {
					res.MatrixBundle[i].Branch = planOverrideBranch
				}
				if forceBranch != "" && res.MatrixBundle[i].AppID == localForce {
					res.MatrixBundle[i].Branch = forceBranch
				}
			}
		}

		// Filter computed matrix if flags are explicitly provided
		if len(planArches) > 0 || planBranch != "" {
			archMap := make(map[string]bool)
			for _, a := range planArches {
				archMap[a] = true
			}

			filterRows := func(rows []plan.MatrixRow) []plan.MatrixRow {
				var filtered []plan.MatrixRow
				for _, r := range rows {
					matchArch := len(planArches) == 0 || archMap[r.Arch]
					matchBranch := planBranch == "" || r.Branch == planBranch
					if matchArch && matchBranch {
						filtered = append(filtered, r)
					}
				}
				return filtered
			}

			res.Matrix = filterRows(res.Matrix)
			res.MatrixManifest = filterRows(res.MatrixManifest)
			res.MatrixBundle = filterRows(res.MatrixBundle)
			res.Count = len(res.Matrix)
			res.CountManifest = len(res.MatrixManifest)
			res.CountBundle = len(res.MatrixBundle)

			// Re-collect active app IDs from the filtered matrix
			appIDMap := make(map[string]bool)
			for _, r := range res.Matrix {
				appIDMap[r.AppID] = true
			}
			var filteredApps []string
			for _, id := range res.Apps {
				if appIDMap[id] {
					filteredApps = append(filteredApps, id)
				}
			}
			res.Apps = filteredApps
		}

		if planDisableLinter {
			for i := range res.Matrix {
				res.Matrix[i].RunLinter = false
			}
			for i := range res.MatrixManifest {
				res.MatrixManifest[i].RunLinter = false
			}
			for i := range res.MatrixBundle {
				res.MatrixBundle[i].RunLinter = false
			}
		}

		if outputFormat == "gitlab" {
			buildArm64 := os.Getenv("BUILD_ARM64") == "true"
			yamlContent := plan.GenerateGitLabPipeline(res, buildArm64)
			if planOutputFile != "" && planOutputFile != "-" {
				if err := os.WriteFile(planOutputFile, []byte(yamlContent), 0644); err != nil {
					return NewCmdErrorf(1, "Failed to write GitLab pipeline file: %w", err)
				}
			} else {
				fmt.Print(yamlContent)
			}
			return nil
		}

		var outBytes []byte
		switch outputFormat {
		case "matrix":
			outBytes, err = json.Marshal(res.Matrix)
		case "matrix-manifest":
			outBytes, err = json.Marshal(res.MatrixManifest)
		case "matrix-bundle":
			outBytes, err = json.Marshal(res.MatrixBundle)
		case "apps":
			outBytes, err = json.Marshal(res.Apps)
		default:
			outBytes, err = json.Marshal(res)
		}

		if err != nil {
			return NewCmdErrorf(1, "Failed to marshal output: %w", err)
		}

		fmt.Println(string(outBytes))

		if planOutputFile != "" {
			mustJSON := func(v any) (string, error) {
				b, err := json.Marshal(v)
				if err != nil {
					return "", err
				}
				// Serialize empty lists as `[]` instead of `null` to ensure array outputs.
				if string(b) == "null" {
					return "[]", nil
				}
				return string(b), nil
			}

			appsJSON, err := mustJSON(res.Apps)
			if err != nil {
				return NewCmdErrorf(1, "Failed to marshal apps: %w", err)
			}
			matrixJSON, err := mustJSON(res.Matrix)
			if err != nil {
				return NewCmdErrorf(1, "Failed to marshal matrix: %w", err)
			}
			matrixManifestJSON, err := mustJSON(res.MatrixManifest)
			if err != nil {
				return NewCmdErrorf(1, "Failed to marshal matrix manifest: %w", err)
			}
			matrixBundleJSON, err := mustJSON(res.MatrixBundle)
			if err != nil {
				return NewCmdErrorf(1, "Failed to marshal matrix bundle: %w", err)
			}

			if err := ciout.Emit(planOutputFile, []ciout.KV{
				{Key: "apps", Value: appsJSON},
				{Key: "matrix", Value: matrixJSON},
				{Key: "matrix-manifest", Value: matrixManifestJSON},
				{Key: "matrix-bundle", Value: matrixBundleJSON},
				{Key: "count", Value: strconv.Itoa(res.Count)},
				{Key: "count-manifest", Value: strconv.Itoa(res.CountManifest)},
				{Key: "count-bundle", Value: strconv.Itoa(res.CountBundle)},
			}); err != nil {
				return NewCmdErrorf(1, "Failed to write output file: %w", err)
			}
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(planCmd)

	planCmd.Flags().StringVar(&baseSHA, "base-sha", "", "git base commit SHA to diff against")
	planCmd.Flags().StringVar(&forceFlag, "force", "", "force selection ('all' or specific app ID)")
	planCmd.Flags().StringVar(&workflowPath, "workflow-path", "", "caller workflow file path (forces rebuild if changed)")
	planCmd.Flags().StringVar(&outputFormat, "output", "json", "output format ('json', 'matrix', 'matrix-manifest', 'matrix-bundle', 'apps')")
	planCmd.Flags().StringVar(&planOutputFile, "output-file", "", "write all plan keys as dotenv KEY=VALUE (- or empty = stdout)")
	planCmd.Flags().StringVar(&planManifest, "manifest", "", "path to a single flatpak manifest file (bypasses config file)")
	planCmd.Flags().StringSliceVar(&planArches, "arch", nil, "architectures to limit target build matrix to (e.g. x86_64, aarch64)")
	planCmd.Flags().StringVar(&planBranch, "branch", "", "branch/channel to use (defaults to stable)")
	planCmd.Flags().StringVar(&planOverrideBranch, "override-branch", "", "override branch/channel for all planned apps")
	planCmd.Flags().BoolVar(&planDisableLinter, "disable-linter", false, "disable linting for all planned apps in the matrix")
}
