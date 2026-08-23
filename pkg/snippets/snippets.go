package snippets

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/scm"
)

var (
	appIDRegexp      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)
	branchRegexp     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	remoteNameRegexp = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	validURLPattern  = regexp.MustCompile(`^https?://[a-zA-Z0-9.\-_/:]+$`)
)

func isValidAppID(s string) bool {
	return appIDRegexp.MatchString(s) && !strings.Contains(s, "..")
}

func isValidChannel(s string) bool {
	return branchRegexp.MatchString(s) && !strings.Contains(s, "..")
}

func isValidRemoteName(s string) bool {
	return remoteNameRegexp.MatchString(s) && !strings.Contains(s, "..")
}

func isValidURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && validURLPattern.MatchString(raw)
}

// SnippetOptions configures the snippet generator.
type SnippetOptions struct {
	Format     string // "markdown", "md", "html", "json"
	AppID      string // Optional app filter
	Channel    string // Optional channel/branch filter
	RemoteName string // Override remote name
	PagesURL   string // Override pages URL
	NoSign     *bool  // Explicit override for unsigned repository commands
}

// RepositoryInfo contains metadata and commands for setting up the Flatpak remote.
type RepositoryInfo struct {
	RemoteName   string `json:"remote_name"`
	RepoTitle    string `json:"repo_title"`
	PagesURL     string `json:"pages_url"`
	RepoFileURL  string `json:"repo_file_url"`
	IsSigned     bool   `json:"is_signed"`
	RemoteAddCmd string `json:"remote_add_cmd"`
}

// ChannelSnippet represents the install, run, and one-click instructions for a specific channel.
type ChannelSnippet struct {
	Channel              string `json:"channel"`
	IsDefault            bool   `json:"is_default"`
	InstallCmd           string `json:"install_cmd"`
	RunCmd               string `json:"run_cmd"`
	MakeCurrentCmd       string `json:"make_current_cmd"`
	FlatpakrefURL        string `json:"flatpakref_url"`
	FlatpakrefInstallCmd string `json:"flatpakref_install_cmd"`
}

// AppSnippets holds all snippet metadata for a specific application.
type AppSnippets struct {
	AppID            string           `json:"app_id"`
	Channels         []ChannelSnippet `json:"channels"`
	SwitchChannelCmd string           `json:"switch_channel_cmd,omitempty"`
}

// SnippetResult is the top-level container holding resolved repository and application snippets.
type SnippetResult struct {
	Repo RepositoryInfo `json:"repo"`
	Apps []AppSnippets  `json:"apps"`
}

// GenerateSnippets resolves configuration and generates installation snippets.
func GenerateSnippets(cfg *config.Config, opts SnippetOptions) (*SnippetResult, error) {
	if opts.AppID != "" && !isValidAppID(opts.AppID) {
		return nil, fmt.Errorf("invalid application ID %q: must start with an alphanumeric character and contain only alphanumeric characters, dots, underscores, or hyphens", opts.AppID)
	}
	if opts.Channel != "" && !isValidChannel(opts.Channel) {
		return nil, fmt.Errorf("invalid channel %q: must contain only alphanumeric characters, dots, underscores, or hyphens", opts.Channel)
	}

	remoteName := opts.RemoteName
	if remoteName == "" && cfg != nil {
		remoteName = cfg.RemoteName
	}
	if remoteName == "" {
		remoteName = scm.RemoteName()
	}
	if remoteName == "" {
		remoteName = "aetherpak"
	}
	if !isValidRemoteName(remoteName) {
		return nil, fmt.Errorf("invalid remote name %q: must start with an alphanumeric character and contain only alphanumeric characters, dots, underscores, or hyphens", remoteName)
	}

	repoTitle := ""
	if cfg != nil {
		repoTitle = cfg.RepoTitle
	}
	if repoTitle == "" {
		repoTitle = remoteName
	}

	pagesURL := opts.PagesURL
	if pagesURL == "" && cfg != nil {
		pagesURL = cfg.PagesURL
	}
	if pagesURL == "" {
		pagesURL = scm.PagesURL()
	}
	if pagesURL == "" {
		return nil, fmt.Errorf("pages_url is required (specify via --pages-url or configure 'pages_url' in aetherpak.yaml)")
	}
	pagesURL = strings.TrimSuffix(pagesURL, "/")
	if !isValidURL(pagesURL) {
		return nil, fmt.Errorf("invalid pages_url %q: must be a valid http or https URL without query parameters or shell metacharacters", pagesURL)
	}

	noSign := false
	if opts.NoSign != nil {
		noSign = *opts.NoSign
	} else if cfg != nil {
		noSign = cfg.NoSign
	}

	repoFileURL := fmt.Sprintf("%s/%s.flatpakrepo", pagesURL, remoteName)

	var remoteAddCmd string
	if noSign {
		remoteAddCmd = fmt.Sprintf("flatpak remote-add --if-not-exists --user --no-gpg-verify %s %s", remoteName, repoFileURL)
	} else {
		remoteAddCmd = fmt.Sprintf("flatpak remote-add --if-not-exists --user %s %s", remoteName, repoFileURL)
	}

	repoInfo := RepositoryInfo{
		RemoteName:   remoteName,
		RepoTitle:    repoTitle,
		PagesURL:     pagesURL,
		RepoFileURL:  repoFileURL,
		IsSigned:     !noSign,
		RemoteAddCmd: remoteAddCmd,
	}

	// Resolve configured or default apps
	type appChannelSet struct {
		channels map[string]bool
	}
	appMap := make(map[string]*appChannelSet)
	var appOrder []string

	if cfg != nil && len(cfg.Apps) > 0 {
		appFound := false
		appMatchedChannel := false

		for _, app := range cfg.Apps {
			if app.ID == "" {
				continue
			}
			if opts.AppID != "" && app.ID != opts.AppID {
				continue
			}
			if opts.AppID != "" && app.ID == opts.AppID {
				appFound = true
			}

			branch := app.Branch
			if branch == "" {
				branch = "stable"
			}
			if opts.Channel != "" && branch != opts.Channel {
				continue
			}
			appMatchedChannel = true

			set, exists := appMap[app.ID]
			if !exists {
				set = &appChannelSet{channels: make(map[string]bool)}
				appMap[app.ID] = set
				appOrder = append(appOrder, app.ID)
			}
			set.channels[branch] = true
		}

		if opts.AppID != "" && !appFound {
			return nil, fmt.Errorf("application %q not found in configuration", opts.AppID)
		}
		if opts.AppID != "" && opts.Channel != "" && !appMatchedChannel {
			return nil, fmt.Errorf("channel %q is not configured for application %q", opts.Channel, opts.AppID)
		}
		if opts.Channel != "" && len(appMap) == 0 {
			return nil, fmt.Errorf("no applications found matching channel %q", opts.Channel)
		}
	}

	// Zero-config or fallback if no apps configured in config
	if len(appMap) == 0 {
		appID := "org.example.App"
		if opts.AppID != "" {
			appID = opts.AppID
		}
		zeroChannels := make(map[string]bool)
		if opts.Channel != "" {
			zeroChannels[opts.Channel] = true
		} else {
			zeroChannels["stable"] = true
			zeroChannels["nightly"] = true
		}
		appMap[appID] = &appChannelSet{
			channels: zeroChannels,
		}
		appOrder = append(appOrder, appID)
	}

	var resultApps []AppSnippets

	for _, appID := range appOrder {
		if !isValidAppID(appID) {
			return nil, fmt.Errorf("invalid application ID %q: must start with an alphanumeric character and contain only alphanumeric characters, dots, underscores, or hyphens", appID)
		}
		set := appMap[appID]

		var channels []string
		for ch := range set.channels {
			if !isValidChannel(ch) {
				return nil, fmt.Errorf("invalid channel %q: must contain only alphanumeric characters, dots, underscores, or hyphens", ch)
			}
			channels = append(channels, ch)
		}
		sortChannels(channels)

		var channelSnippets []ChannelSnippet
		for _, ch := range channels {
			isDefault := ch == "stable"
			refName := fmt.Sprintf("%s-%s.flatpakref", appID, strings.ReplaceAll(ch, "/", "-"))
			refURL := fmt.Sprintf("%s/refs/%s", pagesURL, refName)

			installCmd := fmt.Sprintf("flatpak install --user %s %s//%s", remoteName, appID, ch)
			runCmd := fmt.Sprintf("flatpak run %s//%s", appID, ch)
			makeCurrentCmd := fmt.Sprintf("flatpak make-current --user %s %s", appID, ch)
			refInstallCmd := fmt.Sprintf("flatpak install --user %s", refURL)

			channelSnippets = append(channelSnippets, ChannelSnippet{
				Channel:              ch,
				IsDefault:            isDefault,
				InstallCmd:           installCmd,
				RunCmd:               runCmd,
				MakeCurrentCmd:       makeCurrentCmd,
				FlatpakrefURL:        refURL,
				FlatpakrefInstallCmd: refInstallCmd,
			})
		}

		var switchCmd string
		if len(channelSnippets) > 1 {
			// Provide example of switching to the second channel (e.g. nightly)
			otherCh := channelSnippets[1].Channel
			switchCmd = fmt.Sprintf("flatpak install --user %s %s//%s && flatpak make-current --user %s %s", remoteName, appID, otherCh, appID, otherCh)
		}

		resultApps = append(resultApps, AppSnippets{
			AppID:            appID,
			Channels:         channelSnippets,
			SwitchChannelCmd: switchCmd,
		})
	}

	return &SnippetResult{
		Repo: repoInfo,
		Apps: resultApps,
	}, nil
}

// sortChannels sorts channel names with standard ordering: stable first, beta, nightly, then alphabetically.
func sortChannels(channels []string) {
	priority := func(name string) int {
		switch strings.ToLower(name) {
		case "stable":
			return 1
		case "beta":
			return 2
		case "nightly":
			return 3
		default:
			return 4
		}
	}

	sort.Slice(channels, func(i, j int) bool {
		pi := priority(channels[i])
		pj := priority(channels[j])
		if pi != pj {
			return pi < pj
		}
		return channels[i] < channels[j]
	})
}
