package cmd

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Unset SCM/CI environment variables to prevent autodetection
	// during CLI command validation and flag tests.
	vars := []string{
		"GITHUB_ACTIONS",
		"GITHUB_REPOSITORY",
		"GITHUB_REPOSITORY_OWNER",
		"GITHUB_ACTOR",
		"GITLAB_CI",
		"CI_REGISTRY",
		"CI_REGISTRY_IMAGE",
		"CI_PROJECT_PATH",
		"CI_PAGES_URL",
		"CI_REGISTRY_USER",
		"CI_JOB_TOKEN",
		"CI_PROJECT_PATH_SLUG",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}

	os.Exit(m.Run())
}
