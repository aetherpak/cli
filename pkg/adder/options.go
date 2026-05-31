package adder

import "github.com/aetherpak/aetherpak/pkg/config"

// Option describes a configurable add option surfaced in both the wizard
// checklist and as a CLI flag, with a tooltip, and how it maps onto a
// config.App when enabled. The registry below is the single source of truth:
// adding an entry makes the option appear as a checkbox, a CLI flag, and a
// help string automatically.
type Option struct {
	Key       string            // kebab key -> CLI flag name + wizard key
	Label     string            // checklist label
	Help      string            // tooltip + flag usage text
	Default   bool              // default enabled state
	AppliesTo func(Source) bool // builder-affecting options skip prebuilt bundles
	Apply     func(*config.App) // mutation applied when the option is enabled
}

// builtSrc reports whether a source produces a build from source (manifest or
// git), as opposed to a prebuilt bundle.
func builtSrc(s Source) bool { return s != SourceBundle }

// appendArg appends arg to args only if it is not already present.
func appendArg(args []string, arg string) []string {
	for _, a := range args {
		if a == arg {
			return args
		}
	}
	return append(args, arg)
}

// BoolOptions is the registry of boolean add options.
var BoolOptions = []Option{
	{
		Key:       "run-linter",
		Label:     "Run linter",
		Help:      "Run flatpak-builder-lint before and after the build",
		Default:   false,
		AppliesTo: builtSrc,
		Apply:     func(a *config.App) { a.RunLinter = true },
	},
	{
		Key:       "install-deps-from-flathub",
		Label:     "Install deps from flathub",
		Help:      "Append --install-deps-from=flathub to the builder args",
		Default:   true,
		AppliesTo: builtSrc,
		Apply: func(a *config.App) {
			a.BuilderArgs = appendArg(a.BuilderArgs, "--install-deps-from=flathub")
		},
	},
	{
		Key:       "ccache",
		Label:     "Enable ccache",
		Help:      "Cache C/C++ compilation between builds",
		Default:   false,
		AppliesTo: builtSrc,
		Apply:     func(a *config.App) { t := true; a.CCache = &t },
	},
}

// applyOptions applies the enabled registry options (filtered by source) and any
// free-form builder args to app. Free-form builder args are only meaningful for
// built sources and are de-duplicated.
func applyOptions(app *config.App, src Source, toggles map[string]bool, builderArgs []string) {
	for _, o := range BoolOptions {
		if o.AppliesTo(src) && toggles[o.Key] {
			o.Apply(app)
		}
	}
	if builtSrc(src) {
		for _, arg := range builderArgs {
			app.BuilderArgs = appendArg(app.BuilderArgs, arg)
		}
	}
}
