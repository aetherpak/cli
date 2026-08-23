package snippets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"
)

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

func sanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || !validURLPattern.MatchString(rawURL) {
		return "#"
	}
	return html.EscapeString(rawURL)
}

// FormatMarkdown formats SnippetResult as GitHub-flavored Markdown.
func FormatMarkdown(res *SnippetResult) (string, error) {
	if res == nil {
		return "", fmt.Errorf("snippet result is nil")
	}

	var sb strings.Builder

	title := res.Repo.RepoTitle
	if title == "" {
		title = res.Repo.RemoteName
	}
	sb.WriteString(fmt.Sprintf("# Flatpak Installation Guide - %s\n\n", title))

	// Repository remote setup section
	sb.WriteString("## 1. Add Repository Remote\n\n")
	sb.WriteString("Add the Flatpak remote repository to your system:\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString(res.Repo.RemoteAddCmd + "\n")
	sb.WriteString("```\n\n")

	// Apps section
	sb.WriteString("## 2. Install Applications\n\n")

	multiApp := len(res.Apps) > 1

	for _, app := range res.Apps {
		if multiApp {
			sb.WriteString(fmt.Sprintf("### Application: `%s`\n\n", app.AppID))
		}

		for _, ch := range app.Channels {
			chTitle := titleCase(ch.Channel)
			refURL := sanitizeURL(ch.FlatpakrefURL)
			if multiApp {
				sb.WriteString(fmt.Sprintf("#### %s Channel (`%s`)\n\n", chTitle, ch.Channel))
				sb.WriteString("##### One-Click Install (.flatpakref)\n")
			} else {
				sb.WriteString(fmt.Sprintf("### %s Channel (`%s`)\n\n", chTitle, ch.Channel))
				sb.WriteString("#### One-Click Install (.flatpakref)\n")
			}
			sb.WriteString(fmt.Sprintf("Download the [%s .flatpakref file](%s) and open it with your software manager, or install via CLI:\n\n", chTitle, refURL))
			sb.WriteString("```bash\n")
			sb.WriteString(ch.FlatpakrefInstallCmd + "\n")
			sb.WriteString("```\n\n")

			if multiApp {
				sb.WriteString("##### CLI Install\n\n")
			} else {
				sb.WriteString("#### CLI Install\n\n")
			}
			sb.WriteString("```bash\n")
			sb.WriteString(ch.InstallCmd + "\n")
			sb.WriteString("```\n\n")

			if multiApp {
				sb.WriteString("##### Run Application\n\n")
			} else {
				sb.WriteString("#### Run Application\n\n")
			}
			sb.WriteString("```bash\n")
			sb.WriteString(ch.RunCmd + "\n")
			sb.WriteString("```\n\n")
		}

		// Channel switching guide if multiple channels exist
		if len(app.Channels) > 1 {
			if multiApp {
				sb.WriteString("#### Channel Management\n\n")
			} else {
				sb.WriteString("### Channel Management\n\n")
			}
			sb.WriteString("If you have multiple channels installed side-by-side, configure the default active branch:\n\n")
			sb.WriteString("```bash\n")
			for _, ch := range app.Channels {
				sb.WriteString(fmt.Sprintf("# Set %s as active default\n%s\n\n", ch.Channel, ch.MakeCurrentCmd))
			}
			sb.WriteString("```\n\n")
		}
	}

	return strings.TrimSpace(sb.String()) + "\n", nil
}

// FormatHTML formats SnippetResult as semantic, embeddable HTML.
func FormatHTML(res *SnippetResult) (string, error) {
	if res == nil {
		return "", fmt.Errorf("snippet result is nil")
	}

	var sb strings.Builder

	title := html.EscapeString(res.Repo.RepoTitle)
	if title == "" {
		title = html.EscapeString(res.Repo.RemoteName)
	}

	sb.WriteString(`<div class="aetherpak-snippets">` + "\n")
	sb.WriteString(fmt.Sprintf("  <h2>Installation Guide: %s</h2>\n\n", title))

	// Remote setup
	sb.WriteString("  <section class=\"aetherpak-section aetherpak-remote-setup\">\n")
	sb.WriteString("    <h3>1. Add Repository Remote</h3>\n")
	sb.WriteString("    <p>Add the Flatpak remote repository before installing applications:</p>\n")
	sb.WriteString("    <pre><code class=\"language-bash\">" + html.EscapeString(res.Repo.RemoteAddCmd) + "</code></pre>\n")
	sb.WriteString("  </section>\n\n")

	// Apps
	sb.WriteString("  <section class=\"aetherpak-section aetherpak-apps\">\n")
	sb.WriteString("    <h3>2. Install Applications</h3>\n")

	multiApp := len(res.Apps) > 1

	for _, app := range res.Apps {
		appIDEsc := html.EscapeString(app.AppID)
		sb.WriteString("    <div class=\"aetherpak-app-card\">\n")
		if multiApp {
			sb.WriteString(fmt.Sprintf("      <h4><code>%s</code></h4>\n", appIDEsc))
		}

		for _, ch := range app.Channels {
			chEsc := html.EscapeString(ch.Channel)
			chTitleEsc := html.EscapeString(titleCase(ch.Channel))
			refURLEsc := sanitizeURL(ch.FlatpakrefURL)

			sb.WriteString(fmt.Sprintf("      <div class=\"aetherpak-channel aetherpak-channel-%s\">\n", chEsc))
			if multiApp {
				sb.WriteString(fmt.Sprintf("        <h5>%s Channel (<code>%s</code>)</h5>\n", chTitleEsc, chEsc))
			} else {
				sb.WriteString(fmt.Sprintf("        <h4>%s Channel (<code>%s</code>)</h4>\n", chTitleEsc, chEsc))
			}

			// Flatpakref button
			sb.WriteString(fmt.Sprintf("        <p><a class=\"btn-flatpakref\" href=\"%s\" download>Download %s .flatpakref</a></p>\n", refURLEsc, chTitleEsc))

			// Install CLI
			sb.WriteString("        <p>Install via command-line:</p>\n")
			sb.WriteString("        <pre><code class=\"language-bash\">" + html.EscapeString(ch.InstallCmd) + "</code></pre>\n")

			// Run CLI
			sb.WriteString("        <p>Run:</p>\n")
			sb.WriteString("        <pre><code class=\"language-bash\">" + html.EscapeString(ch.RunCmd) + "</code></pre>\n")
			sb.WriteString("      </div>\n")
		}

		if len(app.Channels) > 1 {
			sb.WriteString("      <div class=\"aetherpak-channel-switch\">\n")
			if multiApp {
				sb.WriteString("        <h5>Switch Active Channel</h5>\n")
			} else {
				sb.WriteString("        <h4>Switch Active Channel</h4>\n")
			}
			sb.WriteString("        <pre><code class=\"language-bash\">")
			for _, ch := range app.Channels {
				sb.WriteString(fmt.Sprintf("# Set %s as active\n%s\n", html.EscapeString(ch.Channel), html.EscapeString(ch.MakeCurrentCmd)))
			}
			sb.WriteString("</code></pre>\n")
			sb.WriteString("      </div>\n")
		}

		sb.WriteString("    </div>\n")
	}

	sb.WriteString("  </section>\n")
	sb.WriteString("</div>\n")

	return sb.String(), nil
}

// FormatJSON formats SnippetResult as indented JSON.
func FormatJSON(res *SnippetResult) (string, error) {
	if res == nil {
		return "", fmt.Errorf("snippet result is nil")
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		return "", fmt.Errorf("failed to encode snippets as JSON: %w", err)
	}

	return buf.String(), nil
}
