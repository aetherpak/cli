package status

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/config"
	"github.com/aetherpak/aetherpak/pkg/executil"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/signing"
	"github.com/charmbracelet/lipgloss"
)

// DependencyInfo holds info about a single system executable check.
type DependencyInfo struct {
	Name     string
	Found    bool
	Path     string
	Version  string
	Required bool
	Message  string
}

// AppDiagnostic holds validation details for a single configured app.
type AppDiagnostic struct {
	AppID string
	Type  string // "manifest" or "bundle"
	Valid bool
	Error string
}

// Report aggregates all diagnostic check results.
type Report struct {
	Dependencies []DependencyInfo
	ConfigPath   string
	ConfigLoaded bool
	ConfigError  error
	Registry     string
	PagesURL     string
	Apps         []AppDiagnostic

	SigningEnabled bool
	GPGKeysCount   int
	Fingerprint    string
	SigningError   error
	PassphraseOk   bool
}

// Check run diagnostics and returns a Report.
func Check(
	executor executil.Executor,
	cfg *config.Config,
	configErr error,
	configPath string,
	gpgKeys []string,
	passphrase []byte,
) Report {
	if executor == nil {
		executor = executil.NewOSExecutor()
	}

	report := Report{
		ConfigPath:   configPath,
		ConfigLoaded: cfg != nil && configErr == nil,
		ConfigError:  configErr,
	}

	// 1. Check dependencies
	deps := []struct {
		name     string
		required bool
		args     []string
	}{
		{"flatpak", true, []string{"--version"}},
		{"flatpak-builder", true, []string{"--version"}},
		{"ostree", true, []string{"--version"}},
		{"flatpak-builder-lint", false, []string{"--version"}},
		{"podman", false, []string{"--version"}},
		{"docker", false, []string{"--version"}},
	}

	for _, d := range deps {
		info := DependencyInfo{
			Name:     d.name,
			Required: d.required,
		}

		path, err := executor.LookPath(d.name)
		if err == nil {
			info.Found = true
			info.Path = path
			info.Version = getVersion(executor, d.name, d.args...)
		} else if d.name == "flatpak-builder-lint" {
			// Fallback: check if flatpak is available and org.flatpak.Builder can run linter
			if _, fErr := executor.LookPath("flatpak"); fErr == nil {
				lintVer := getVersion(executor, "flatpak", "run", "--command=flatpak-builder-lint", "org.flatpak.Builder", "--version")
				if lintVer != "" {
					info.Found = true
					info.Path = "flatpak run org.flatpak.Builder"
					info.Version = lintVer
				}
			}
		}

		report.Dependencies = append(report.Dependencies, info)
	}

	// 2. Validate configuration options
	if cfg != nil {
		report.Registry = cfg.Registry
		report.PagesURL = cfg.PagesURL
		report.SigningEnabled = !cfg.NoSign

		for _, app := range cfg.Apps {
			diag := AppDiagnostic{
				AppID: app.ID,
			}
			if app.Manifest != "" {
				diag.Type = "manifest"
				// Check if manifest file exists
				if _, err := os.Stat(app.Manifest); err != nil {
					diag.Valid = false
					diag.Error = fmt.Sprintf("Manifest file not found: %s", app.Manifest)
				} else {
					diag.Valid = true
				}
			} else {
				diag.Type = "bundle"
				diag.Valid = true
			}
			report.Apps = append(report.Apps, diag)
		}
	} else {
		// Zero-config defaults GPG signing to enabled
		report.SigningEnabled = true
	}

	if report.Registry == "" {
		report.Registry = os.Getenv("AETHERPAK_REGISTRY")
	}
	if report.PagesURL == "" {
		report.PagesURL = os.Getenv("AETHERPAK_PAGES_URL")
	}

	// 3. Check GPG signing
	// Gather raw keys from inputs or environment fallback
	rawKeys := gpgKeys
	if len(rawKeys) == 0 {
		envKey := os.Getenv("AETHERPAK_GPG_KEY")
		if envKey != "" {
			rawKeys = []string{envKey}
		}
	}

	keys := signing.ResolveKeys(rawKeys)

	report.GPGKeysCount = len(keys)

	if len(keys) > 0 {
		signer, err := signing.NewSigner(keys, passphrase)
		if err != nil {
			report.SigningError = err
			report.PassphraseOk = false
		} else {
			report.PassphraseOk = true
			report.Fingerprint = signer.Fingerprint()
		}
	}

	return report
}

func getVersion(executor executil.Executor, name string, args ...string) string {
	cmd := executor.Command(name, args...)
	var stdout bytes.Buffer
	cmd.SetStdout(&stdout)
	if err := cmd.Run(); err != nil {
		return ""
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "unknown"
	}
	lines := strings.Split(out, "\n")
	firstLine := strings.TrimSpace(lines[0])
	if strings.HasSuffix(firstLine, ":") && len(lines) > 1 {
		secondLine := strings.TrimSpace(lines[1])
		if strings.HasPrefix(secondLine, "Version:") {
			versionVal := strings.TrimPrefix(secondLine, "Version:")
			versionVal = strings.Trim(strings.TrimSpace(versionVal), "'\"")
			return firstLine + " " + versionVal
		}
	}
	return firstLine
}

// PrintReport writes a formatted summary of the Report to w.
func PrintReport(w io.Writer, r Report) {
	plain := logger.IsPlain()

	// Lipgloss styles
	var (
		titleStyle  lipgloss.Style
		borderStyle lipgloss.Style
		successIcon string
		errorIcon   string
		warnIcon    string
	)

	if plain {
		successIcon = "✔ "
		errorIcon = "✖ "
		warnIcon = "[!] "
	} else {
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
		borderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
		successIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("✔ ")
		errorIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✖ ")
		warnIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render("[!] ")
	}

	// 1. Dependencies Section
	printHeader(w, "System Dependencies", plain, titleStyle, borderStyle)

	flatpakBuilderFound := false
	containerToolAvailable := false
	containerToolName := ""

	for _, dep := range r.Dependencies {
		statusIcon := successIcon
		statusText := "Found"
		if !dep.Found {
			statusIcon = errorIcon
			statusText = "NOT found in PATH"
			if dep.Required {
				statusText += " (Required)"
			}
		} else {
			if dep.Name == "flatpak-builder" {
				flatpakBuilderFound = true
			}
			if dep.Name == "podman" || dep.Name == "docker" {
				containerToolAvailable = true
				if containerToolName == "" {
					containerToolName = dep.Name
				}
			}

			statusText = fmt.Sprintf("Found (%s, version %s)", dep.Path, dep.Version)
		}
		fmt.Fprintf(w, "  %s %-20s %s\n", statusIcon, dep.Name, statusText)
	}
	printFooter(w, plain, borderStyle)

	// Recommendation if flatpak-builder is missing but podman/docker is available
	if !flatpakBuilderFound && containerToolAvailable {
		fmt.Fprintf(w, "\n%sRecommendation: To compile applications without flatpak-builder installed\n", warnIcon)
		fmt.Fprintln(w, "    locally, you can run AetherPak using the pre-baked builder container:")
		fmt.Fprintln(w, "")
		fmt.Fprintf(w, "    %s run --privileged --rm -v $(pwd):/workspace -w /workspace \\\n", containerToolName)
		fmt.Fprintln(w, "      ghcr.io/aetherpak/cli:latest-builder aetherpak build --manifest <manifest>")
		fmt.Fprintln(w, "")
	}

	// 2. Configuration Section
	printHeader(w, "Configuration Status", plain, titleStyle, borderStyle)
	if r.ConfigLoaded {
		fmt.Fprintf(w, "  %s Config File         Found (%s)\n", successIcon, r.ConfigPath)
	} else {
		if r.ConfigError != nil {
			fmt.Fprintf(w, "  %s Config File         Failed to load (%s)\n", errorIcon, r.ConfigPath)
			fmt.Fprintf(w, "                         Error: %v\n", r.ConfigError)
		} else {
			fmt.Fprintf(w, "  %s Config File         Not found (using zero-config defaults)\n", warnIcon)
		}
	}

	if r.Registry != "" {
		fmt.Fprintf(w, "  %s OCI Registry        %s\n", successIcon, r.Registry)
	} else {
		fmt.Fprintf(w, "  %s OCI Registry        Missing (AETHERPAK_REGISTRY or config 'registry' required for OCI publish)\n", errorIcon)
	}

	if r.PagesURL != "" {
		fmt.Fprintf(w, "  %s Pages URL           %s\n", successIcon, r.PagesURL)
	} else {
		fmt.Fprintf(w, "  %s Pages URL           Missing (AETHERPAK_PAGES_URL or config 'pages_url' required for site build)\n", errorIcon)
	}

	if r.ConfigLoaded {
		manifestAppsCount := 0
		bundleAppsCount := 0
		for _, app := range r.Apps {
			if app.Type == "manifest" {
				manifestAppsCount++
			} else {
				bundleAppsCount++
			}
		}

		appStatus := successIcon
		for _, app := range r.Apps {
			if !app.Valid {
				appStatus = errorIcon
				break
			}
		}
		fmt.Fprintf(w, "  %s Configured Apps       %d app(s) configured:\n", appStatus, len(r.Apps))
		for _, app := range r.Apps {
			validText := ""
			statusAppIcon := successIcon
			if !app.Valid {
				statusAppIcon = errorIcon
				validText = fmt.Sprintf(" [Error: %s]", app.Error)
			}
			fmt.Fprintf(w, "                           %s %s (%s-based)%s\n", statusAppIcon, app.AppID, app.Type, validText)
		}
	}
	printFooter(w, plain, borderStyle)

	// 3. Signing Section
	printHeader(w, "GPG Signing & Credentials", plain, titleStyle, borderStyle)
	if !r.SigningEnabled {
		fmt.Fprintf(w, "  %s Signing Mode          Disabled (no_sign: true)\n", warnIcon)
	} else {
		fmt.Fprintf(w, "  %s Signing Mode          Enabled\n", successIcon)
		if r.GPGKeysCount == 0 {
			fmt.Fprintf(w, "  %s GPG Key Status        No GPG keys loaded (runs unsigned if --allow-unsigned is enabled)\n", errorIcon)
		} else {
			if r.SigningError != nil {
				fmt.Fprintf(w, "  %s GPG Key Status        Failed to parse GPG keys\n", errorIcon)
				fmt.Fprintf(w, "                           Error: %v\n", r.SigningError)
			} else {
				fmt.Fprintf(w, "  %s GPG Key Status        Loaded (%d private key(s) parsed successfully)\n", successIcon, r.GPGKeysCount)
				if r.Fingerprint != "" {
					fmt.Fprintf(w, "  %s Primary Fingerprint   %s\n", successIcon, r.Fingerprint)
				}
				if r.PassphraseOk {
					fmt.Fprintf(w, "  %s Passphrase Status     Correct / Unlocked successfully\n", successIcon)
				}
			}
		}
	}
	printFooter(w, plain, borderStyle)
}

func printHeader(w io.Writer, title string, plain bool, titleStyle, borderStyle lipgloss.Style) {
	if plain {
		fmt.Fprintf(w, "┌── %s %s\n", title, strings.Repeat("─", max(1, 70-len(title))))
	} else {
		styledTitle := titleStyle.Render(title)
		borderLen := max(1, 70-len(title))
		fmt.Fprintf(w, "%s%s %s\n", borderStyle.Render("┌── "), styledTitle, borderStyle.Render(strings.Repeat("─", borderLen)))
	}
}

func printFooter(w io.Writer, plain bool, borderStyle lipgloss.Style) {
	if plain {
		fmt.Fprintln(w, "└"+strings.Repeat("─", 74)+"┘")
	} else {
		fmt.Fprintln(w, borderStyle.Render("└"+strings.Repeat("─", 74)+"┘"))
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
