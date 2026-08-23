package appstream

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aetherpak/aetherpak/pkg/executil"
)

var (
	isoDateRegexp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// ResolutionResult contains the resolved release metadata and source provenance.
type ResolutionResult struct {
	Version     string
	Date        string
	Description string
	URL         string
	Source      string
}

// ResolveRelease resolves the active release version and ISO 8601 date from explicit
// flags, CI environment variables, or local git tags.
func ResolveRelease(executor executil.Executor, explicitVersion, explicitDate, explicitDesc, explicitURL string) (ResolutionResult, bool) {
	if executor == nil {
		executor = executil.NewOSExecutor()
	}

	var res ResolutionResult
	res.Description = explicitDesc
	res.URL = explicitURL

	// 1. Explicit CLI / config version
	if explicitVersion != "" {
		res.Version = SanitizeVersion(explicitVersion)
		res.Source = "explicit"
	}

	// 2. CI Environment Variables
	if res.Version == "" {
		if os.Getenv("GITHUB_REF_TYPE") == "tag" && os.Getenv("GITHUB_REF_NAME") != "" {
			res.Version = SanitizeVersion(os.Getenv("GITHUB_REF_NAME"))
			res.Source = "env:GITHUB_REF_NAME"
		} else if ref := os.Getenv("GITHUB_REF"); strings.HasPrefix(ref, "refs/tags/") {
			tag := strings.TrimPrefix(ref, "refs/tags/")
			res.Version = SanitizeVersion(tag)
			res.Source = "env:GITHUB_REF"
		} else if tag := os.Getenv("CI_COMMIT_TAG"); tag != "" {
			res.Version = SanitizeVersion(tag)
			res.Source = "env:CI_COMMIT_TAG"
		} else if tag := os.Getenv("CIRCLE_TAG"); tag != "" {
			res.Version = SanitizeVersion(tag)
			res.Source = "env:CIRCLE_TAG"
		} else if tag := os.Getenv("TRAVIS_TAG"); tag != "" {
			res.Version = SanitizeVersion(tag)
			res.Source = "env:TRAVIS_TAG"
		}
	}

	// 3. Local Git repository tags
	var rawGitTag string
	if res.Version == "" {
		// Attempt exact tag match on HEAD
		cmdExact := executor.Command("git", "describe", "--tags", "--exact-match")
		var outExact bytes.Buffer
		cmdExact.SetStdout(&outExact)
		if err := cmdExact.Run(); err == nil {
			tag := strings.TrimSpace(outExact.String())
			if tag != "" {
				res.Version = SanitizeVersion(tag)
				rawGitTag = tag
				res.Source = "git:exact-tag"
			}
		}

		// Fallback: any closest git tag
		if res.Version == "" {
			cmdAny := executor.Command("git", "describe", "--tags", "--abbrev=0")
			var outAny bytes.Buffer
			cmdAny.SetStdout(&outAny)
			if err := cmdAny.Run(); err == nil {
				tag := strings.TrimSpace(outAny.String())
				if tag != "" {
					res.Version = SanitizeVersion(tag)
					rawGitTag = tag
					res.Source = "git:describe-tag"
				}
			}
		}
	}

	if res.Version == "" {
		return res, false
	}

	// Resolve Date
	if explicitDate != "" {
		res.Date = strings.TrimSpace(explicitDate)
	} else if rawGitTag != "" {
		// Attempt to extract git tag commit date
		cmdDate := executor.Command("git", "log", "-1", "--format=%cs", rawGitTag)
		var outDate bytes.Buffer
		cmdDate.SetStdout(&outDate)
		if err := cmdDate.Run(); err == nil {
			dateStr := strings.TrimSpace(outDate.String())
			if isoDateRegexp.MatchString(dateStr) {
				res.Date = dateStr
			}
		}
	}

	// Default date to current UTC day
	if res.Date == "" {
		res.Date = time.Now().UTC().Format("2006-01-02")
	}

	return res, true
}
