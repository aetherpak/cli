package cmd

import (
	"fmt"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/adder"
	"github.com/aetherpak/aetherpak/pkg/importer"
	"github.com/charmbracelet/huh"
)

// runWizard drives the interactive onboarding form and returns resolved options.
// It only prompts for things that cannot be auto-detected: the app id is always
// derived from the manifest (never asked), and the git manifest path is asked
// for only if auto-detection fails (via opts.PromptManifest).
func runWizard(configPath string) (adder.Options, error) {
	var sourceChoice string
	srcForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What do you want to add?").
				Options(
					huh.NewOption("Local manifest path", "manifest"),
					huh.NewOption("Bundle URL (download & fingerprint)", "bundle"),
					huh.NewOption("Git repo (add as submodule)", "git"),
				).
				Value(&sourceChoice),
		),
	)
	if err := srcForm.Run(); err != nil {
		return adder.Options{}, err
	}

	branch := addBranch
	if branch == "" {
		branch = "stable"
	}
	arches := addArches
	if len(arches) == 0 {
		arches = []string{adder.DefaultArch()}
	}

	opts := adder.Options{
		ConfigPath:    configPath,
		Branch:        branch,
		Arches:        arches,
		Progress:      barProgress(),
		Fetch:         importer.Fetch,
		Confirm:       addConfirm,
		ID:            addID,
		ManifestPath:  addManifest,
		BundleURL:     addBundleURL,
		GitURL:        addGit,
		SubmodulePath: addSubmodulePath,
		BundleSHA256:  addBundleSHA256,
	}

	var sourceForm *huh.Form
	switch sourceChoice {
	case "manifest":
		opts.Source = adder.SourceManifest
		sourceForm = manifestGroup(&opts)
	case "bundle":
		opts.Source = adder.SourceBundle
		sourceForm = bundleGroup(&opts)
	case "git":
		opts.Source = adder.SourceGit
		opts.PromptManifest = promptManifestPath
		sourceForm = gitGroup(&opts)
	default:
		return adder.Options{}, fmt.Errorf("no source selected")
	}
	if err := sourceForm.Run(); err != nil {
		return adder.Options{}, err
	}

	if err := collectOptions(&opts); err != nil {
		return adder.Options{}, err
	}
	return opts, nil
}

func manifestGroup(opts *adder.Options) *huh.Form {
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Manifest path").
			Description("Path to the local Flatpak manifest; the app id is read from it").
			Value(&opts.ManifestPath).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("Branch").
			Description("Flatpak release channel, e.g. stable or beta — not the git branch").
			Value(&opts.Branch),
	))
}

func bundleGroup(opts *adder.Options) *huh.Form {
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Bundle URL").Value(&opts.BundleURL).
			Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("App ID").
			Description("Reverse-DNS id; required because it cannot be derived from a URL").
			Value(&opts.ID).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("Branch").
			Description("Flatpak release channel, e.g. stable or beta — not the git branch").
			Value(&opts.Branch),
	))
}

func gitGroup(opts *adder.Options) *huh.Form {
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Git repository URL").Value(&opts.GitURL).
			Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("Submodule path").
			Description("Blank = manifests/<reponame>").
			Value(&opts.SubmodulePath),
		huh.NewInput().Title("Branch").
			Description("Flatpak release channel, e.g. stable or beta — not the git branch").
			Value(&opts.Branch),
	))
}

// collectOptions shows the boolean option checklist (built from the registry,
// filtered to the source) plus a free-form builder-args input, and records the
// result on opts. Bundles have no applicable options, so the form is skipped.
func collectOptions(opts *adder.Options) error {
	var checkboxes []huh.Option[string]
	for _, o := range adder.BoolOptions {
		if !o.AppliesTo(opts.Source) {
			continue
		}
		flagVal := o.Default
		if v, exists := addToggleFlags[o.Key]; exists {
			flagVal = *v
		}
		label := fmt.Sprintf("%s — %s", o.Label, o.Help)
		checkboxes = append(checkboxes, huh.NewOption(label, o.Key).Selected(flagVal))
	}
	if len(checkboxes) == 0 {
		return nil
	}

	var selected []string
	var builderArgs string
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Options").
			Description("space to toggle, enter to continue").
			Options(checkboxes...).
			Value(&selected),
		huh.NewInput().
			Title("Additional builder args").
			Description("Optional, space-separated (e.g. --jobs=4); args with embedded spaces need editing in aetherpak.yaml").
			Value(&builderArgs),
	))
	if err := form.Run(); err != nil {
		return err
	}

	opts.Toggles = make(map[string]bool, len(checkboxes))
	for _, cb := range checkboxes {
		opts.Toggles[cb.Value] = false
	}
	for _, key := range selected {
		opts.Toggles[key] = true
	}
	opts.BuilderArgs = strings.Fields(builderArgs)
	return nil
}

// promptManifestPath asks for a manifest path within a cloned repo when it could
// not be auto-detected.
func promptManifestPath(repoDir string) (string, error) {
	var p string
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Manifest path within the repo").
			Description("Could not auto-detect a manifest in " + repoDir).
			Value(&p).Validate(huh.ValidateNotEmpty()),
	)).Run()
	return p, err
}
