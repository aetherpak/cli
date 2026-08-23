package snippets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/config"
)

func TestGenerateSnippets_DefaultZeroConfig(t *testing.T) {
	opts := SnippetOptions{
		Format:     "markdown",
		RemoteName: "myrepo",
		PagesURL:   "https://example.github.io/myrepo",
	}

	res, err := GenerateSnippets(nil, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Repo.RemoteName != "myrepo" {
		t.Errorf("expected remote name myrepo, got %s", res.Repo.RemoteName)
	}
	if res.Repo.PagesURL != "https://example.github.io/myrepo" {
		t.Errorf("expected pages URL https://example.github.io/myrepo, got %s", res.Repo.PagesURL)
	}
	if res.Repo.RepoFileURL != "https://example.github.io/myrepo/myrepo.flatpakrepo" {
		t.Errorf("expected repo file URL https://example.github.io/myrepo/myrepo.flatpakrepo, got %s", res.Repo.RepoFileURL)
	}
	if !res.Repo.IsSigned {
		t.Errorf("expected default to be signed")
	}
	if !strings.Contains(res.Repo.RemoteAddCmd, "myrepo.flatpakrepo") {
		t.Errorf("expected remote add command to reference flatpakrepo, got %s", res.Repo.RemoteAddCmd)
	}

	if len(res.Apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(res.Apps))
	}
	app := res.Apps[0]
	if app.AppID != "org.example.App" {
		t.Errorf("expected app ID org.example.App, got %s", app.AppID)
	}
	if len(app.Channels) != 2 {
		t.Fatalf("expected 2 channels (stable, nightly), got %d", len(app.Channels))
	}
	if app.Channels[0].Channel != "stable" || app.Channels[1].Channel != "nightly" {
		t.Errorf("expected stable and nightly channels, got %s and %s", app.Channels[0].Channel, app.Channels[1].Channel)
	}
}

func TestGenerateSnippets_ConfiguredApps(t *testing.T) {
	cfg := &config.Config{
		RemoteName: "flathub-custom",
		RepoTitle:  "Custom Flathub Mirror",
		PagesURL:   "https://pages.domain.org/repo/",
		NoSign:     true,
		Apps: []config.App{
			{
				ID:     "org.gnome.Sudoku",
				Branch: "stable",
			},
			{
				ID:     "org.gnome.Sudoku",
				Branch: "nightly",
			},
			{
				ID:     "org.gnome.Calculator",
				Branch: "beta",
			},
		},
	}

	opts := SnippetOptions{}
	res, err := GenerateSnippets(cfg, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Repo.RemoteName != "flathub-custom" {
		t.Errorf("expected remote name flathub-custom, got %s", res.Repo.RemoteName)
	}
	if res.Repo.RepoTitle != "Custom Flathub Mirror" {
		t.Errorf("expected repo title 'Custom Flathub Mirror', got %s", res.Repo.RepoTitle)
	}
	if res.Repo.PagesURL != "https://pages.domain.org/repo" {
		t.Errorf("expected trailing slash removed from pages URL, got %s", res.Repo.PagesURL)
	}
	if res.Repo.IsSigned {
		t.Errorf("expected unsigned because NoSign is true")
	}
	if !strings.Contains(res.Repo.RemoteAddCmd, "--no-gpg-verify") || !strings.Contains(res.Repo.RemoteAddCmd, "flathub-custom.flatpakrepo") {
		t.Errorf("expected --no-gpg-verify and .flatpakrepo in remote add command, got %s", res.Repo.RemoteAddCmd)
	}

	if len(res.Apps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(res.Apps))
	}

	sudoku := res.Apps[0]
	if sudoku.AppID != "org.gnome.Sudoku" {
		t.Errorf("expected first app org.gnome.Sudoku, got %s", sudoku.AppID)
	}
	if len(sudoku.Channels) != 2 {
		t.Fatalf("expected 2 channels for Sudoku, got %d", len(sudoku.Channels))
	}
	if sudoku.Channels[0].Channel != "stable" {
		t.Errorf("expected first channel stable, got %s", sudoku.Channels[0].Channel)
	}
	if sudoku.Channels[1].Channel != "nightly" {
		t.Errorf("expected second channel nightly, got %s", sudoku.Channels[1].Channel)
	}

	calc := res.Apps[1]
	if calc.AppID != "org.gnome.Calculator" {
		t.Errorf("expected second app org.gnome.Calculator, got %s", calc.AppID)
	}
	if len(calc.Channels) != 1 || calc.Channels[0].Channel != "beta" {
		t.Errorf("expected exactly 1 channel (beta) for Calculator without auto-nightly expansion, got %+v", calc.Channels)
	}
}

func TestGenerateSnippets_AppAndChannelFilter(t *testing.T) {
	cfg := &config.Config{
		RemoteName: "myrepo",
		PagesURL:   "https://example.org",
		Apps: []config.App{
			{
				ID:     "org.gnome.Sudoku",
				Branch: "stable",
			},
			{
				ID:     "org.gnome.Calculator",
				Branch: "nightly",
			},
		},
	}

	opts := SnippetOptions{
		AppID:   "org.gnome.Calculator",
		Channel: "nightly",
	}

	res, err := GenerateSnippets(cfg, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Apps) != 1 {
		t.Fatalf("expected 1 app after filter, got %d", len(res.Apps))
	}
	app := res.Apps[0]
	if app.AppID != "org.gnome.Calculator" {
		t.Errorf("expected org.gnome.Calculator, got %s", app.AppID)
	}
	if len(app.Channels) != 1 {
		t.Fatalf("expected 1 channel after filter, got %d", len(app.Channels))
	}
	if app.Channels[0].Channel != "nightly" {
		t.Errorf("expected channel nightly, got %s", app.Channels[0].Channel)
	}
}

func TestGenerateSnippets_DualFilterChannelMismatch(t *testing.T) {
	cfg := &config.Config{
		RemoteName: "myrepo",
		PagesURL:   "https://example.org",
		Apps: []config.App{
			{
				ID:     "org.gnome.Sudoku",
				Branch: "stable",
			},
		},
	}

	opts := SnippetOptions{
		AppID:   "org.gnome.Sudoku",
		Channel: "nightly",
	}

	_, err := GenerateSnippets(cfg, opts)
	if err == nil {
		t.Fatalf("expected error for channel mismatch on existing app, got nil")
	}
	if !strings.Contains(err.Error(), "channel \"nightly\" is not configured for application \"org.gnome.Sudoku\"") {
		t.Errorf("expected specific channel mismatch error, got: %v", err)
	}
}

func TestGenerateSnippets_ChannelNotFoundInConfig(t *testing.T) {
	cfg := &config.Config{
		RemoteName: "myrepo",
		PagesURL:   "https://example.org",
		Apps: []config.App{
			{
				ID:     "org.gnome.Sudoku",
				Branch: "stable",
			},
		},
	}

	opts := SnippetOptions{
		Channel: "nightly",
	}

	_, err := GenerateSnippets(cfg, opts)
	if err == nil {
		t.Fatalf("expected error for nonexistent channel in config, got nil")
	}
	if !strings.Contains(err.Error(), "matching channel") {
		t.Errorf("expected error message to contain 'matching channel', got: %v", err)
	}
}

func TestGenerateSnippets_MissingPagesURL(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "")
	t.Setenv("CI_PAGES_URL", "")

	cfg := &config.Config{
		RemoteName: "myrepo",
	}

	opts := SnippetOptions{}

	_, err := GenerateSnippets(cfg, opts)
	if err == nil {
		t.Fatalf("expected error when pages_url is empty, got nil")
	}
	if !strings.Contains(err.Error(), "pages_url is required") {
		t.Errorf("expected 'pages_url is required' error, got: %v", err)
	}
}

func TestGenerateSnippets_InvalidPagesURL(t *testing.T) {
	invalidURLs := []string{
		"javascript:alert(1)",
		"ftp://example.com/repo",
		"https://example.com/repo?x=1&y=2",
		"https://example.com/repo;rm -rf /",
	}

	for _, u := range invalidURLs {
		cfg := &config.Config{
			RemoteName: "myrepo",
			PagesURL:   u,
		}
		opts := SnippetOptions{}
		_, err := GenerateSnippets(cfg, opts)
		if err == nil {
			t.Errorf("expected error for invalid pages_url %q, got nil", u)
		}
		if !strings.Contains(err.Error(), "invalid pages_url") {
			t.Errorf("expected 'invalid pages_url' error for %q, got: %v", u, err)
		}
	}
}

func TestGenerateSnippets_InvalidIdentifiers(t *testing.T) {
	cfg := &config.Config{
		PagesURL: "https://example.org",
	}

	invalidTests := []SnippetOptions{
		{RemoteName: "my repo; evil"},
		{RemoteName: "../evil"},
		{RemoteName: "-rf"},
		{AppID: "org.example.App; rm -rf /"},
		{AppID: "../evil"},
		{AppID: "-rf"},
		{Channel: "stable && evil"},
		{Channel: "../evil"},
		{Channel: "-rf"},
	}

	for _, opt := range invalidTests {
		_, err := GenerateSnippets(cfg, opt)
		if err == nil {
			t.Errorf("expected error for invalid identifier in %+v, got nil", opt)
		}
	}
}

func TestGenerateSnippets_ExplicitNoSignOverride(t *testing.T) {
	cfg := &config.Config{
		RemoteName: "myrepo",
		PagesURL:   "https://example.org",
		NoSign:     true,
		Apps: []config.App{
			{
				ID:     "org.gnome.Sudoku",
				Branch: "stable",
			},
		},
	}

	falseVal := false
	opts := SnippetOptions{
		NoSign: &falseVal,
	}

	res, err := GenerateSnippets(cfg, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Repo.IsSigned {
		t.Errorf("expected IsSigned to be true when explicitly overridden with NoSign=false")
	}
	if strings.Contains(res.Repo.RemoteAddCmd, "--no-gpg-verify") {
		t.Errorf("expected signed command without --no-gpg-verify, got: %s", res.Repo.RemoteAddCmd)
	}
}

func TestGenerateSnippets_MissingAppFilterError(t *testing.T) {
	cfg := &config.Config{
		RemoteName: "myrepo",
		PagesURL:   "https://example.org",
		Apps: []config.App{
			{
				ID:     "org.gnome.Sudoku",
				Branch: "stable",
			},
		},
	}

	opts := SnippetOptions{
		AppID: "com.nonexistent.App",
	}

	_, err := GenerateSnippets(cfg, opts)
	if err == nil {
		t.Fatalf("expected error for nonexistent app filter, got nil")
	}
	if !strings.Contains(err.Error(), "not found in configuration") {
		t.Errorf("expected error message to contain 'not found in configuration', got: %v", err)
	}
}

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/refs/app.flatpakref", "https://example.com/refs/app.flatpakref"},
		{"http://example.com/repo.flatpakrepo", "http://example.com/repo.flatpakrepo"},
		{"javascript:alert(1)", "#"},
		{"data:text/html,test", "#"},
		{"", "#"},
	}

	for _, tt := range tests {
		actual := sanitizeURL(tt.input)
		if actual != tt.expected {
			t.Errorf("sanitizeURL(%q) = %q, expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestFormatMarkdown(t *testing.T) {
	res := &SnippetResult{
		Repo: RepositoryInfo{
			RemoteName:   "testrepo",
			RepoTitle:    "Test Repository",
			PagesURL:     "https://test.io",
			RepoFileURL:  "https://test.io/testrepo.flatpakrepo",
			IsSigned:     true,
			RemoteAddCmd: "flatpak remote-add --if-not-exists --user testrepo https://test.io/testrepo.flatpakrepo",
		},
		Apps: []AppSnippets{
			{
				AppID: "org.example.App",
				Channels: []ChannelSnippet{
					{
						Channel:              "stable",
						IsDefault:            true,
						InstallCmd:           "flatpak install --user testrepo org.example.App//stable",
						RunCmd:               "flatpak run org.example.App//stable",
						MakeCurrentCmd:       "flatpak make-current --user org.example.App stable",
						FlatpakrefURL:        "https://test.io/refs/org.example.App-stable.flatpakref",
						FlatpakrefInstallCmd: "flatpak install --user https://test.io/refs/org.example.App-stable.flatpakref",
					},
				},
			},
		},
	}

	md, err := FormatMarkdown(res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(md, "# Flatpak Installation Guide - Test Repository") {
		t.Errorf("expected title header in markdown, got:\n%s", md)
	}
	if !strings.Contains(md, "flatpak remote-add --if-not-exists --user testrepo") {
		t.Errorf("expected remote add cmd in markdown, got:\n%s", md)
	}
	if !strings.Contains(md, "### Stable Channel (`stable`)") {
		t.Errorf("expected ### for single-app channel header, got:\n%s", md)
	}

	// Test multi-app uses #### for channels and ##### for subheadings
	resMulti := &SnippetResult{
		Repo: RepositoryInfo{
			RemoteName:   "testrepo",
			RepoTitle:    "Test Repository",
			PagesURL:     "https://test.io",
			RepoFileURL:  "https://test.io/testrepo.flatpakrepo",
			IsSigned:     true,
			RemoteAddCmd: "flatpak remote-add --if-not-exists --user testrepo https://test.io/testrepo.flatpakrepo",
		},
		Apps: []AppSnippets{
			{
				AppID: "org.example.App1",
				Channels: []ChannelSnippet{
					{Channel: "stable"},
				},
			},
			{
				AppID: "org.example.App2",
				Channels: []ChannelSnippet{
					{Channel: "nightly"},
				},
			},
		},
	}

	mdMulti, err := FormatMarkdown(resMulti)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(mdMulti, "### Application: `org.example.App1`") {
		t.Errorf("expected ### for app header in multi-app, got:\n%s", mdMulti)
	}
	if !strings.Contains(mdMulti, "#### Stable Channel (`stable`)") {
		t.Errorf("expected #### for channel header in multi-app, got:\n%s", mdMulti)
	}
}

func TestFormatHTML(t *testing.T) {
	// Test single-app uses <h4> for channels
	resSingle := &SnippetResult{
		Repo: RepositoryInfo{
			RemoteName:   "testrepo",
			RepoTitle:    "Test Repository",
			PagesURL:     "https://test.io",
			RepoFileURL:  "https://test.io/testrepo.flatpakrepo",
			IsSigned:     true,
			RemoteAddCmd: "flatpak remote-add --if-not-exists --user testrepo https://test.io/testrepo.flatpakrepo",
		},
		Apps: []AppSnippets{
			{
				AppID: "org.example.App",
				Channels: []ChannelSnippet{
					{
						Channel:              "stable",
						IsDefault:            true,
						InstallCmd:           "flatpak install --user testrepo org.example.App//stable",
						RunCmd:               "flatpak run org.example.App//stable",
						MakeCurrentCmd:       "flatpak make-current --user org.example.App stable",
						FlatpakrefURL:        "https://test.io/refs/org.example.App-stable.flatpakref",
						FlatpakrefInstallCmd: "flatpak install --user https://test.io/refs/org.example.App-stable.flatpakref",
					},
				},
			},
		},
	}

	htmlOutSingle, err := FormatHTML(resSingle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(htmlOutSingle, "<div class=\"aetherpak-snippets\">") {
		t.Errorf("expected root container div in HTML, got:\n%s", htmlOutSingle)
	}
	if !strings.Contains(htmlOutSingle, "<h4>Stable Channel (<code>stable</code>)</h4>") {
		t.Errorf("expected <h4> for single-app channel header, got:\n%s", htmlOutSingle)
	}

	// Test multi-app uses <h4> for apps and <h5> for channels
	resMulti := &SnippetResult{
		Repo: RepositoryInfo{
			RemoteName:   "testrepo",
			RepoTitle:    "Test Repository",
			PagesURL:     "https://test.io",
			RepoFileURL:  "https://test.io/testrepo.flatpakrepo",
			IsSigned:     true,
			RemoteAddCmd: "flatpak remote-add --if-not-exists --user testrepo https://test.io/testrepo.flatpakrepo",
		},
		Apps: []AppSnippets{
			{
				AppID: "org.example.App1",
				Channels: []ChannelSnippet{
					{
						Channel: "stable",
					},
				},
			},
			{
				AppID: "org.example.App2",
				Channels: []ChannelSnippet{
					{
						Channel: "nightly",
					},
				},
			},
		},
	}

	htmlOutMulti, err := FormatHTML(resMulti)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(htmlOutMulti, "<h4><code>org.example.App1</code></h4>") {
		t.Errorf("expected <h4> for app header in multi-app, got:\n%s", htmlOutMulti)
	}
	if !strings.Contains(htmlOutMulti, "<h5>Stable Channel (<code>stable</code>)</h5>") {
		t.Errorf("expected <h5> for channel header in multi-app, got:\n%s", htmlOutMulti)
	}
}

func TestFormatJSON(t *testing.T) {
	res := &SnippetResult{
		Repo: RepositoryInfo{
			RemoteName:   "testrepo",
			RepoTitle:    "Test Repository",
			PagesURL:     "https://test.io",
			RepoFileURL:  "https://test.io/testrepo.flatpakrepo",
			IsSigned:     true,
			RemoteAddCmd: "flatpak remote-add --if-not-exists --user testrepo https://test.io/testrepo.flatpakrepo",
		},
		Apps: []AppSnippets{
			{
				AppID: "org.example.App",
				Channels: []ChannelSnippet{
					{
						Channel:              "stable",
						IsDefault:            true,
						InstallCmd:           "flatpak install --user testrepo org.example.App//stable",
						RunCmd:               "flatpak run org.example.App//stable",
						MakeCurrentCmd:       "flatpak make-current --user org.example.App stable",
						FlatpakrefURL:        "https://test.io/refs/org.example.App-stable.flatpakref",
						FlatpakrefInstallCmd: "flatpak install --user https://test.io/refs/org.example.App-stable.flatpakref",
					},
				},
			},
		},
	}

	jsonOut, err := FormatJSON(res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed SnippetResult
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("failed to parse generated JSON: %v", err)
	}

	if parsed.Repo.RemoteName != "testrepo" {
		t.Errorf("expected parsed remote name testrepo, got %s", parsed.Repo.RemoteName)
	}
	if len(parsed.Apps) != 1 || parsed.Apps[0].AppID != "org.example.App" {
		t.Errorf("expected parsed app org.example.App, got %+v", parsed.Apps)
	}
}
