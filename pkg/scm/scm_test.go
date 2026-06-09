package scm

import (
	"os"
	"testing"
)

func TestSCMAutodetectGitHub(t *testing.T) {
	// Setup env
	os.Setenv("GITHUB_ACTIONS", "true")
	os.Setenv("GITHUB_REPOSITORY", "Owner/Repo-Name")
	os.Setenv("GITHUB_REPOSITORY_OWNER", "Owner")
	os.Setenv("GITHUB_ACTOR", "octocat")
	defer func() {
		os.Unsetenv("GITHUB_ACTIONS")
		os.Unsetenv("GITHUB_REPOSITORY")
		os.Unsetenv("GITHUB_REPOSITORY_OWNER")
		os.Unsetenv("GITHUB_ACTOR")
	}()

	if provider := DetectProvider(); provider != GitHub {
		t.Errorf("expected provider GitHub, got %v", provider)
	}
	if reg := Registry(); reg != "ghcr.io" {
		t.Errorf("expected Registry ghcr.io, got %q", reg)
	}
	if repo := OCIRepository(); repo != "owner/repo-name" {
		t.Errorf("expected OCIRepository owner/repo-name, got %q", repo)
	}
	if url := PagesURL(); url != "https://owner.github.io/repo-name" {
		t.Errorf("expected PagesURL https://owner.github.io/repo-name, got %q", url)
	}
	if user := Username(); user != "octocat" {
		t.Errorf("expected Username octocat, got %q", user)
	}
	if pass := Password(); pass != "" {
		t.Errorf("expected empty Password on GitHub Actions, got %q", pass)
	}
	if remote := RemoteName(); remote != "owner-repo-name" {
		t.Errorf("expected RemoteName owner-repo-name, got %q", remote)
	}
}

func TestSCMAutodetectGitLab(t *testing.T) {
	// Setup env
	os.Setenv("GITLAB_CI", "true")
	os.Setenv("CI_REGISTRY", "registry.example.com")
	os.Setenv("CI_REGISTRY_IMAGE", "registry.example.com/some-group/sub-group/my-project")
	os.Setenv("CI_PAGES_URL", "https://some-group.pages.example.com/my-project")
	os.Setenv("CI_REGISTRY_USER", "gitlab-ci-token")
	os.Setenv("CI_JOB_TOKEN", "mock-job-token")
	os.Setenv("CI_PROJECT_PATH_SLUG", "some-group-my-project")
	defer func() {
		os.Unsetenv("GITLAB_CI")
		os.Unsetenv("CI_REGISTRY")
		os.Unsetenv("CI_REGISTRY_IMAGE")
		os.Unsetenv("CI_PAGES_URL")
		os.Unsetenv("CI_REGISTRY_USER")
		os.Unsetenv("CI_JOB_TOKEN")
		os.Unsetenv("CI_PROJECT_PATH_SLUG")
	}()

	if provider := DetectProvider(); provider != GitLab {
		t.Errorf("expected provider GitLab, got %v", provider)
	}
	if reg := Registry(); reg != "registry.example.com" {
		t.Errorf("expected Registry registry.example.com, got %q", reg)
	}
	if repo := OCIRepository(); repo != "some-group/sub-group/my-project" {
		t.Errorf("expected OCIRepository some-group/sub-group/my-project, got %q", repo)
	}
	if url := PagesURL(); url != "https://some-group.pages.example.com/my-project" {
		t.Errorf("expected PagesURL https://some-group.pages.example.com/my-project, got %q", url)
	}
	if user := Username(); user != "gitlab-ci-token" {
		t.Errorf("expected Username gitlab-ci-token, got %q", user)
	}
	if pass := Password(); pass != "mock-job-token" {
		t.Errorf("expected Password mock-job-token, got %q", pass)
	}
	if remote := RemoteName(); remote != "some-group-my-project" {
		t.Errorf("expected RemoteName some-group-my-project, got %q", remote)
	}
}

func TestSCMAutodetectUnknown(t *testing.T) {
	if provider := DetectProvider(); provider != Unknown {
		t.Errorf("expected Unknown provider, got %v", provider)
	}
	if reg := Registry(); reg != "" {
		t.Errorf("expected empty Registry, got %q", reg)
	}
	if repo := OCIRepository(); repo != "" {
		t.Errorf("expected empty OCIRepository, got %q", repo)
	}
	if url := PagesURL(); url != "" {
		t.Errorf("expected empty PagesURL, got %q", url)
	}
}
