package scm

import (
	"os"
	"strings"
)

type Provider string

const (
	GitHub  Provider = "github"
	GitLab  Provider = "gitlab"
	Unknown Provider = "unknown"
)

// DetectProvider returns the current SCM/CI provider based on environment variables.
func DetectProvider() Provider {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return GitHub
	}
	if os.Getenv("GITLAB_CI") == "true" {
		return GitLab
	}
	return Unknown
}

// Registry returns the default registry host for the current provider.
func Registry() string {
	switch DetectProvider() {
	case GitHub:
		return "ghcr.io"
	case GitLab:
		if r := os.Getenv("CI_REGISTRY"); r != "" {
			return r
		}
		return "registry.gitlab.com"
	}
	return ""
}

// OCIRepository returns the default repository path (without registry host).
func OCIRepository() string {
	switch DetectProvider() {
	case GitHub:
		return strings.ToLower(os.Getenv("GITHUB_REPOSITORY"))
	case GitLab:
		regImage := os.Getenv("CI_REGISTRY_IMAGE")
		registry := os.Getenv("CI_REGISTRY")
		if regImage != "" && registry != "" {
			prefix := registry + "/"
			if strings.HasPrefix(regImage, prefix) {
				return strings.ToLower(strings.TrimPrefix(regImage, prefix))
			}
		}
		if projPath := os.Getenv("CI_PROJECT_PATH"); projPath != "" {
			return strings.ToLower(projPath)
		}
	}
	return ""
}

// PagesURL returns the default Pages hosting URL.
func PagesURL() string {
	switch DetectProvider() {
	case GitHub:
		owner := os.Getenv("GITHUB_REPOSITORY_OWNER")
		repo := os.Getenv("GITHUB_REPOSITORY")
		if owner != "" && repo != "" {
			parts := strings.Split(repo, "/")
			if len(parts) == 2 {
				return "https://" + strings.ToLower(owner) + ".github.io/" + strings.ToLower(parts[1])
			}
		}
	case GitLab:
		return os.Getenv("CI_PAGES_URL")
	}
	return ""
}

// Username returns default OCI username.
func Username() string {
	switch DetectProvider() {
	case GitHub:
		return os.Getenv("GITHUB_ACTOR")
	case GitLab:
		return os.Getenv("CI_REGISTRY_USER")
	}
	return ""
}

// Password returns default OCI password/token.
func Password() string {
	switch DetectProvider() {
	case GitLab:
		return os.Getenv("CI_JOB_TOKEN")
	}
	return ""
}

// RemoteName returns the default remote name (slugified).
func RemoteName() string {
	switch DetectProvider() {
	case GitHub:
		repo := os.Getenv("GITHUB_REPOSITORY")
		if repo != "" {
			return sanitizeRemoteName(repo)
		}
	case GitLab:
		if slug := os.Getenv("CI_PROJECT_PATH_SLUG"); slug != "" {
			return slug
		}
		if path := os.Getenv("CI_PROJECT_PATH"); path != "" {
			return sanitizeRemoteName(path)
		}
	}
	return ""
}

func sanitizeRemoteName(name string) string {
	name = strings.ToLower(name)
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	res := sb.String()
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	res = strings.Trim(res, "-_")
	return res
}
