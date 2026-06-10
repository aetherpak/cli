package cmd

import (
	"fmt"
	"os"
	"regexp"
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

func parseFlatpakRemotes(remotes []string) (map[string]config.RemoteConfig, error) {
	out := make(map[string]config.RemoteConfig)
	for _, r := range remotes {
		parts := strings.SplitN(r, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid flatpak remote format %q, must be name=url", r)
		}
		out[parts[0]] = config.RemoteConfig{URL: parts[1]}
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

var nonAlphaNumRegexp = regexp.MustCompile(`[^a-z0-9_-]`)

func sanitizeRemoteName(name string) string {
	name = strings.ToLower(name)
	return nonAlphaNumRegexp.ReplaceAllString(name, "-")
}

// SplitAndCleanSlice splits each string in the slice by comma and newline,
// and trims whitespace around each resulting element.
func SplitAndCleanSlice(slice []string) []string {
	var cleaned []string
	for _, s := range slice {
		s = strings.ReplaceAll(s, "\r", "")
		s = strings.ReplaceAll(s, "\n", ",")
		parts := strings.Split(s, ",")
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
	}
	return cleaned
}

// parseAppIDRef parses an app-id reference of the format "app-id//branch".
// It returns the clean app ID and optionally the branch name if present.
func parseAppIDRef(ref string) (string, string) {
	if parts := strings.SplitN(ref, "//", 2); len(parts) == 2 {
		return parts[0], parts[1]
	}
	return ref, ""
}
