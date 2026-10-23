package cmd

import (
	"fmt"

	"github.com/aetherpak/aetherpak/pkg/ciout"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/oci"
	"github.com/aetherpak/aetherpak/pkg/scm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	deltasPruneRegistry      string
	deltasPruneOCIRepository string
	deltasPruneInsecure      bool
	deltasPruneDryRun        bool
	deltasPruneOutputFile    string
)

var deltasCmd = &cobra.Command{
	Use:   "deltas",
	Short: "Manage published OCI image deltas",
	Args:  cobra.NoArgs,
}

var deltasPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete stale delta and source tags",
	Args:  cobra.NoArgs,
	Long:  `Removes _delta-* and _deltasrc-* tags whose target or source manifest returns a definitive 404, then rebuilds _deltaindex from the remaining delta tags. Auth, network, and other transient errors leave tags untouched.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return NewCmdErrorf(2, "Configuration error: %w", err)
		}

		registry := deltasPruneRegistry
		if registry == "" {
			registry = cfg.Registry
		}
		if registry == "" {
			registry = scm.Registry()
		}
		if registry == "" {
			return NewCmdError(2, fmt.Errorf("registry is required"))
		}

		ociRepository := deltasPruneOCIRepository
		if ociRepository == "" {
			ociRepository = cfg.OCIRepository
		}
		if ociRepository == "" {
			ociRepository = scm.OCIRepository()
		}
		if ociRepository == "" {
			return NewCmdError(2, fmt.Errorf("oci repository is required"))
		}

		res, err := oci.PruneDeltas(oci.PruneOptions{
			Registry:      registry,
			OCIRepository: ociRepository,
			Insecure:      deltasPruneInsecure,
			OCIUsername:   viper.GetString("oci_username"),
			OCIPassword:   viper.GetString("oci_password"),
			DryRun:        deltasPruneDryRun,
		})
		if err != nil {
			return NewCmdError(1, err)
		}

		if err := ciout.Emit(deltasPruneOutputFile, []ciout.KV{
			{Key: "delta-tags", Value: fmt.Sprintf("%d", res.DeltaTags)},
			{Key: "source-tags", Value: fmt.Sprintf("%d", res.SourceTags)},
			{Key: "index-rebuilt", Value: fmt.Sprintf("%t", res.IndexRebuilt)},
		}); err != nil {
			return NewCmdError(1, err)
		}

		verb := "Removed"
		if deltasPruneDryRun {
			verb = "Would remove"
		}
		logger.SuccessBanner("Delta Prune Completed", fmt.Sprintf("%s %d delta tag(s) and %d source tag(s).", verb, res.DeltaTags, res.SourceTags))
		return nil
	},
}

func init() {
	RootCmd.AddCommand(deltasCmd)
	deltasCmd.AddCommand(deltasPruneCmd)

	deltasPruneCmd.Flags().StringVar(&deltasPruneRegistry, "registry", "", "target OCI registry host")
	deltasPruneCmd.Flags().StringVar(&deltasPruneOCIRepository, "oci-repository", "", "target repository path/name")
	deltasPruneCmd.Flags().BoolVar(&deltasPruneInsecure, "insecure", false, "allow connection to an insecure OCI registry (HTTP)")
	deltasPruneCmd.Flags().BoolVar(&deltasPruneDryRun, "dry-run", false, "report what would be removed without deleting anything")
	deltasPruneCmd.Flags().StringVar(&deltasPruneOutputFile, "output-file", "", "write resolved outputs as dotenv KEY=VALUE (- or empty = stdout)")
}
