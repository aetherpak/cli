package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/config"
)

// resolveLinterExceptions retrieves the final set of linter exceptions and exceptions file,
// merging configuration defaults/overrides and applying environment variables and CLI flag overrides.
func resolveLinterExceptions(
	linterExceptionsFileChanged bool,
	linterExceptionChanged bool,
	defaultExceptions []string,
	defaultExceptionsFile string,
	flagExceptions []string,
	flagExceptionsFile string,
) ([]string, string) {
	exceptions := defaultExceptions
	exceptionsFile := defaultExceptionsFile

	if envVal := os.Getenv("AETHERPAK_LINTER_EXCEPTIONS_FILE"); envVal != "" {
		exceptionsFile = envVal
	} else if envVal := os.Getenv("AETHERPAK_LINTER_EXCEPTIONS"); envVal != "" {
		if strings.HasSuffix(envVal, ".json") {
			exceptionsFile = envVal
		} else {
			var envList []string
			for _, item := range strings.Split(envVal, ",") {
				item = strings.TrimSpace(item)
				if item != "" {
					envList = append(envList, item)
				}
			}
			if len(envList) > 0 {
				exceptions = envList
			}
		}
	}

	if linterExceptionsFileChanged {
		exceptionsFile = flagExceptionsFile
	}
	if linterExceptionChanged {
		exceptions = flagExceptions
	}

	return exceptions, exceptionsFile
}

func parseFlatpakRemotes(remotes []string) (map[string]string, error) {
	out := make(map[string]string)
	for _, r := range remotes {
		parts := strings.SplitN(r, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid flatpak remote format %q, must be name=url", r)
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

func parseFlatpakDeps(deps []string) ([]config.FlatpakDep, error) {
	var out []config.FlatpakDep
	for _, d := range deps {
		var parts []string
		if strings.Contains(d, ":") {
			parts = strings.SplitN(d, ":", 2)
		} else if strings.Contains(d, "=") {
			parts = strings.SplitN(d, "=", 2)
		} else {
			return nil, fmt.Errorf("invalid flatpak dependency format %q, must be remote:ref", d)
		}
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid flatpak dependency format %q, must be remote:ref", d)
		}
		out = append(out, config.FlatpakDep{
			Remote: parts[0],
			Ref:    parts[1],
		})
	}
	return out, nil
}
