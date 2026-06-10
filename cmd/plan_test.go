package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/plan"
	"github.com/spf13/viper"
)

func TestPlanOverrideBranchAndForceRef(t *testing.T) {
	t.Chdir(t.TempDir())

	configData := []byte(`
apps:
  - id: org.example.AppOne
    manifest: apps/one.json
    runtime: gnome-50
  - id: org.example.AppTwo
    manifest: apps/two.json
    runtime: gnome-50
`)
	err := os.WriteFile("aetherpak.yaml", configData, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	defer func() {
		planManifest = ""
		forceFlag = ""
		planArches = nil
		planBranch = ""
		planOverrideBranch = ""
		planOutputFile = ""
		planCmd.Flags().Lookup("manifest").Changed = false
		planCmd.Flags().Lookup("force").Changed = false
		planCmd.Flags().Lookup("arch").Changed = false
		planCmd.Flags().Lookup("branch").Changed = false
		planCmd.Flags().Lookup("override-branch").Changed = false
		planCmd.Flags().Lookup("output-file").Changed = false
	}()

	t.Run("override-branch overrides all apps", func(t *testing.T) {
		viper.Reset()
		planManifest = ""
		forceFlag = ""
		planArches = nil
		planBranch = ""
		planOverrideBranch = ""
		planOutputFile = ""
		planCmd.Flags().Lookup("manifest").Changed = false
		planCmd.Flags().Lookup("force").Changed = false
		planCmd.Flags().Lookup("arch").Changed = false
		planCmd.Flags().Lookup("branch").Changed = false
		planCmd.Flags().Lookup("override-branch").Changed = false
		planCmd.Flags().Lookup("output-file").Changed = false

		outputFile := filepath.Join(t.TempDir(), "output1.env")

		_ = planCmd.Flags().Set("force", "all")
		_ = planCmd.Flags().Set("override-branch", "beta")
		_ = planCmd.Flags().Set("output-file", outputFile)

		// Mock bindFlags/init config
		initConfig()
		bindFlags(planCmd)

		err = planCmd.RunE(planCmd, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Read output file and parse the matrix JSON from env
		content, err := os.ReadFile(outputFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}

		// Find the matrix line: matrix='...'
		var matrixJSON string
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "matrix=") {
				matrixJSON = strings.Trim(strings.TrimPrefix(line, "matrix="), "'\"")
				break
			}
		}

		if matrixJSON == "" {
			t.Fatal("could not find matrix in output file")
		}

		var matrixRows []plan.MatrixRow
		if err := json.Unmarshal([]byte(matrixJSON), &matrixRows); err != nil {
			t.Fatalf("failed to parse matrix JSON: %v", err)
		}

		if len(matrixRows) != 2 {
			t.Fatalf("expected 2 matrix rows, got %d", len(matrixRows))
		}

		for _, row := range matrixRows {
			if row.Branch != "beta" {
				t.Errorf("expected branch to be beta, got %q for app %s", row.Branch, row.AppID)
			}
		}
	})

	t.Run("force with app-id//branch overrides specific app branch", func(t *testing.T) {
		viper.Reset()
		planManifest = ""
		forceFlag = ""
		planArches = nil
		planBranch = ""
		planOverrideBranch = ""
		planOutputFile = ""
		planCmd.Flags().Lookup("manifest").Changed = false
		planCmd.Flags().Lookup("force").Changed = false
		planCmd.Flags().Lookup("arch").Changed = false
		planCmd.Flags().Lookup("branch").Changed = false
		planCmd.Flags().Lookup("override-branch").Changed = false
		planCmd.Flags().Lookup("output-file").Changed = false

		outputFile := filepath.Join(t.TempDir(), "output2.env")

		_ = planCmd.Flags().Set("force", "org.example.AppOne//beta")
		_ = planCmd.Flags().Set("output-file", outputFile)

		// Mock bindFlags/init config
		initConfig()
		bindFlags(planCmd)

		err = planCmd.RunE(planCmd, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := os.ReadFile(outputFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}

		var matrixJSON string
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "matrix=") {
				matrixJSON = strings.Trim(strings.TrimPrefix(line, "matrix="), "'\"")
				break
			}
		}

		var matrixRows []plan.MatrixRow
		if err := json.Unmarshal([]byte(matrixJSON), &matrixRows); err != nil {
			t.Fatalf("failed to parse matrix JSON: %v", err)
		}

		if len(matrixRows) != 1 {
			t.Fatalf("expected 1 matrix row, got %d", len(matrixRows))
		}

		row := matrixRows[0]
		if row.AppID != "org.example.AppOne" {
			t.Errorf("expected app-id org.example.AppOne, got %q", row.AppID)
		}
		if row.Branch != "beta" {
			t.Errorf("expected branch to be beta, got %q", row.Branch)
		}
	})
}
