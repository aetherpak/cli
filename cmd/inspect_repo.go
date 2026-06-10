package cmd

import (
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
	Short: "Resolve app-id/arch/branch/ref-type from an existing OSTree repo",
	RunE: func(cmd *cobra.Command, args []string) error {
		info, err := repoinfo.Resolve(nil, inspectRepoPath)
		if err != nil {
			return NewCmdError(1, err)
		}
		if err := ciout.Emit(inspectOutputFile, []ciout.KV{
			{Key: "app-id", Value: info.AppID},
			{Key: "branch", Value: info.Branch},
			{Key: "arch", Value: info.Arch},
			{Key: "repo-path", Value: info.RepoPath},
			{Key: "ref-type", Value: info.RefType},
		}); err != nil {
			return NewCmdError(1, err)
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(inspectRepoCmd)
	inspectRepoCmd.Flags().StringVar(&inspectRepoPath, "repo-path", "repo", "path to OSTree repository")
	inspectRepoCmd.Flags().StringVar(&inspectOutputFile, "output-file", "", "write resolved outputs as dotenv KEY=VALUE (- or empty = stdout)")
}
