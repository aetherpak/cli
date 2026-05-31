package cmd

import (
	"fmt"
	"os"

	"github.com/aetherpak/aetherpak/pkg/adder"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	addManifest      string
	addBundleURL     string
	addGit           string
	addGitManifest   string
	addSubmodulePath string
	addID            string
	addBranch        string
	addArches        []string
	addBundleSHA256  string
	addConfirm       bool
	addBuilderArgs   []string
	// addToggleFlags holds the *bool backing each registry option flag, keyed
	// by option Key. Populated in init() from adder.BoolOptions.
	addToggleFlags = map[string]*bool{}
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add an app to aetherpak.yaml from a manifest, bundle URL, or git repo",
	Long: `Creates or modifies an aetherpak.yaml configuration by adding one
application. The source can be a local Flatpak manifest, a remote bundle URL
(downloaded and fingerprinted), or a git repository (added as a submodule and
initialised recursively). Runs an interactive wizard when attached to a TTY;
otherwise reads flags. A diff is shown before changes are written unless
--confirm is given.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath := resolveConfigPath()

		opts, err := resolveAddOptions(configPath, isInteractive())
		if err != nil {
			return NewCmdError(2, err)
		}

		if err := adder.Run(opts); err != nil {
			return NewCmdError(1, err)
		}
		return nil
	},
}

// resolveConfigPath honors --config/AETHERPAK_CONFIG, else detects an existing
// aetherpak.yaml/.yml, else defaults to aetherpak.yaml.
func resolveConfigPath() string {
	if p := viper.GetString("config"); p != "" {
		return p
	}
	for _, candidate := range []string{"aetherpak.yaml", "aetherpak.yml"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "aetherpak.yaml"
}

func isInteractive() bool {
	if logger.IsPlain() {
		return false
	}
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

// resolveAddOptions builds adder.Options from flags, falling back to the
// interactive wizard when no source flag is supplied on a TTY.
func resolveAddOptions(configPath string, interactive bool) (adder.Options, error) {
	src, count := selectedSource()
	if count > 1 {
		return adder.Options{}, fmt.Errorf("only one of --manifest, --bundle-url, --git may be given")
	}

	if count == 0 {
		if interactive {
			opts, err := runWizard(configPath)
			opts.Plain = logger.IsPlain()
			return opts, err
		}
		return adder.Options{}, fmt.Errorf("one of --manifest, --bundle-url, --git is required in non-interactive mode")
	}

	var progress adder.ProgressFunc
	if !logger.IsPlain() {
		progress = barProgress()
	}

	toggles := make(map[string]bool, len(addToggleFlags))
	for key, v := range addToggleFlags {
		toggles[key] = *v
	}

	return adder.Options{
		ConfigPath:    configPath,
		Source:        src,
		ManifestPath:  addManifest,
		BundleURL:     addBundleURL,
		BundleSHA256:  addBundleSHA256,
		GitURL:        addGit,
		GitManifest:   addGitManifest,
		SubmodulePath: addSubmodulePath,
		ID:            addID,
		Branch:        addBranch,
		Arches:        addArches,
		Toggles:       toggles,
		BuilderArgs:   addBuilderArgs,
		Confirm:       addConfirm,
		Plain:         logger.IsPlain(),
		Progress:      progress,
	}, nil
}

func selectedSource() (adder.Source, int) {
	count := 0
	var src adder.Source
	if addManifest != "" {
		src, count = adder.SourceManifest, count+1
	}
	if addBundleURL != "" {
		src, count = adder.SourceBundle, count+1
	}
	if addGit != "" {
		src, count = adder.SourceGit, count+1
	}
	return src, count
}

func init() {
	RootCmd.AddCommand(addCmd)

	addCmd.Flags().StringVar(&addManifest, "manifest", "", "path to a local Flatpak manifest")
	addCmd.Flags().StringVar(&addBundleURL, "bundle-url", "", "URL of a remote .flatpak bundle to download and fingerprint")
	addCmd.Flags().StringVar(&addGit, "git", "", "git repository URL to add as a submodule")
	addCmd.Flags().StringVar(&addGitManifest, "git-manifest", "", "manifest path within the git repo (auto-detected if omitted)")
	addCmd.Flags().StringVar(&addSubmodulePath, "submodule-path", "", "submodule destination path (default: manifests/<reponame>)")
	addCmd.Flags().StringVar(&addID, "id", "", "app ID (reverse-DNS); derived from the manifest when omitted")
	addCmd.Flags().StringVar(&addBranch, "branch", "stable", "published branch channel")
	addCmd.Flags().StringSliceVar(&addArches, "arch", []string{adder.DefaultArch()}, "target architecture(s) (default: host arch)")
	addCmd.Flags().StringVar(&addBundleSHA256, "bundle-sha256", "", "expected SHA-256 of the bundle (verified; computed if omitted)")
	addCmd.Flags().BoolVarP(&addConfirm, "confirm", "y", false, "skip the diff confirmation prompt")
	addCmd.Flags().StringArrayVar(&addBuilderArgs, "builder-arg", nil, "extra flatpak-builder arg (repeatable; built sources only)")

	// One flag per registry option; the wizard derives its checklist from the
	// same source.
	for _, opt := range adder.BoolOptions {
		v := new(bool)
		addCmd.Flags().BoolVar(v, opt.Key, opt.Default, opt.Help)
		addToggleFlags[opt.Key] = v
	}
}
