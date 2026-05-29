package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/plan"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	baseSHA        string
	forceFlag      string
	workflowPath   string
	outputFormat   string
	planOutputFile string
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Computes changes and plans flatpak build matrix",
	Long:  `Computes the diff between git refs and expands aetherpak.yaml to generate a matrix of target apps to build.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}

		configPath := viper.ConfigFileUsed()
		if configPath == "" {
			if cfgFile != "" {
				configPath = cfgFile
			} else {
				configPath = "aetherpak.yaml"
				if _, err := os.Stat("aetherpak.yml"); err == nil {
					configPath = "aetherpak.yml"
				}
			}
		}

		res, err := plan.ComputePlan(cfg, configPath, baseSHA, forceFlag, workflowPath)
		if err != nil {
			return NewCmdErrorf(1, "Plan computation error: %w", err)
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
			mustJSON := func(v any) string {
				b, _ := json.Marshal(v)
				// Serialize empty lists as `[]` instead of `null` to ensure array outputs.
				if string(b) == "null" {
					return "[]"
				}
				return string(b)
			}
			if err := ciout.Emit(planOutputFile, []ciout.KV{
				{Key: "apps", Value: mustJSON(res.Apps)},
				{Key: "matrix", Value: mustJSON(res.Matrix)},
				{Key: "matrix-manifest", Value: mustJSON(res.MatrixManifest)},
				{Key: "matrix-bundle", Value: mustJSON(res.MatrixBundle)},
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
}
