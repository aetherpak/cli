package cmd

import (
	"fmt"
	"os"

	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/repoinfo"
	"github.com/spf13/cobra"
)

var (
	inspectRepoPath   string
	inspectOutputFile string
)

var inspectRepoCmd = &cobra.Command{
	Use:   "inspect-repo",
	Short: "Resolve app-id/arch/branch from an existing OSTree repo",
	Run: func(cmd *cobra.Command, args []string) {
		info, err := repoinfo.Resolve(inspectRepoPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		if err := ciout.Emit(inspectOutputFile, []ciout.KV{
			{Key: "app-id", Value: info.AppID},
			{Key: "branch", Value: info.Branch},
			{Key: "arch", Value: info.Arch},
			{Key: "repo-path", Value: info.RepoPath},
		}); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	},
}

func init() {
	RootCmd.AddCommand(inspectRepoCmd)
	inspectRepoCmd.Flags().StringVar(&inspectRepoPath, "repo-path", "repo", "path to OSTree repository")
	inspectRepoCmd.Flags().StringVar(&inspectOutputFile, "output-file", "", "write resolved outputs as dotenv KEY=VALUE (- or empty = stdout)")
}
