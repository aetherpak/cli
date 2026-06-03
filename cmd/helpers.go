package cmd

import (
	"os"
	"strings"
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
