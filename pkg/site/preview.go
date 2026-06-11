package site

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/record"
)

// PreviewOptions configures the template preview site generator.
type PreviewOptions struct {
	TemplatePath string
	SiteDir      string
	Live         bool
	LiveURL      string
	GPG          bool
	Apps         string // "single" or "multiple"
	PagesURL     string
	RemoteName   string
	RepoTitle    string
	RepoHomepage string
}

// GeneratePreview builds a static site preview in opts.SiteDir.
func GeneratePreview(opts PreviewOptions) error {
	logger.Info("Generating template preview in directory: %s", opts.SiteDir)

	if err := os.MkdirAll(opts.SiteDir, 0755); err != nil {
		return fmt.Errorf("failed to create preview site directory: %w", err)
	}

	var siteOpts SiteOptions
	siteOpts.SiteDir = opts.SiteDir
	siteOpts.LandingPage = true
	siteOpts.IndexTemplate = opts.TemplatePath
	siteOpts.RemoteName = opts.RemoteName
	siteOpts.RepoTitle = opts.RepoTitle
	siteOpts.RepoHomepage = opts.RepoHomepage
	siteOpts.AllowUnsigned = true // Allow unsigned by default if GPG is disabled

	if opts.Live {
		liveURL := opts.LiveURL
		if liveURL == "" {
			liveURL = opts.PagesURL
		}
		if liveURL == "" {
			return fmt.Errorf("live preview requires a pages-url (specify --live-url or configure pages_url)")
		}
		siteOpts.PagesURL = liveURL
		siteOpts.Reconcile = false // Disable reconcile during preview to avoid registry head check latency

		// If GPG is requested for live preview, generate mock GPG key
		if opts.GPG {
			gpgKey, err := generateMockGPGKey()
			if err != nil {
				return err
			}
			siteOpts.GPGKeys = []string{gpgKey}
			siteOpts.NoSign = false
		} else {
			siteOpts.NoSign = true
		}

		// Create an empty temporary directory for records to satisfy BuildSite
		tempRecordsDir, err := os.MkdirTemp("", "aetherpak-preview-records-*")
		if err != nil {
			return fmt.Errorf("failed to create temporary records directory: %w", err)
		}
		defer os.RemoveAll(tempRecordsDir)
		siteOpts.RecordsDir = tempRecordsDir

		return BuildSite(siteOpts)
	}

	// Generating preview using dummy data
	tempRecordsDir, err := os.MkdirTemp("", "aetherpak-preview-records-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary records directory: %w", err)
	}
	defer os.RemoveAll(tempRecordsDir)
	siteOpts.RecordsDir = tempRecordsDir

	if err := generateDummyRecords(tempRecordsDir, opts.Apps); err != nil {
		return fmt.Errorf("failed to generate dummy records: %w", err)
	}

	if opts.GPG {
		gpgKey, err := generateMockGPGKey()
		if err != nil {
			return err
		}
		siteOpts.GPGKeys = []string{gpgKey}
		siteOpts.NoSign = false
	} else {
		siteOpts.NoSign = true
	}

	return BuildSite(siteOpts)
}

// StartPreviewServer starts a local HTTP server serving the site directory.
func StartPreviewServer(siteDir string, port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start listener on %s: %w", addr, err)
	}

	server := &http.Server{
		Addr:    addr,
		Handler: http.FileServer(http.Dir(siteDir)),
	}

	logger.SuccessBanner("Preview Server Running", fmt.Sprintf("Serving preview at: http://%s\nPress Ctrl+C to stop.", addr))

	return server.Serve(listener)
}

func generateMockGPGKey() (string, error) {
	entity, err := openpgp.NewEntity("AetherPak Preview", "Template Preview Key", "preview@aetherpak.local", nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate temporary GPG key: %w", err)
	}

	var privKeyBlock bytes.Buffer
	wPriv, err := armor.Encode(&privKeyBlock, openpgp.PrivateKeyType, nil)
	if err != nil {
		return "", err
	}
	if err := entity.SerializePrivate(wPriv, nil); err != nil {
		return "", err
	}
	wPriv.Close()
	return privKeyBlock.String(), nil
}

func generateDummyRecords(recordsDir string, appsMode string) error {
	// 1. GNOME Builder (stable, beta, x86_64, aarch64)
	app1Stable64 := record.Record{
		AppID:    "org.gnome.Builder",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "gnome/builder",
		Registry: "ghcr.io",
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}
	app1Stable64Labels := map[string]string{
		"org.flatpak.ref":                   "app/org.gnome.Builder/x86_64/stable",
		"org.flatpak.metadata":              "[Application]\nname=org.gnome.Builder\nruntime=org.gnome.Platform/x86_64/45",
		"org.flatpak.timestamp":             "1717200000", // June 1, 2024
		"org.flatpak.installed-size":        "157286400",  // 150 MB
		"org.flatpak.download-size":         "52428800",   // 50 MB
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>GNOME Builder</name><summary>An IDE for writing GNOME applications</summary></component>`,
		"org.freedesktop.appstream.icon-64": "https://raw.githubusercontent.com/GNOME/gnome-builder/main/data/icons/hicolor/scalable/apps/org.gnome.Builder.svg",
	}
	if _, err := record.WriteRecord(recordsDir, app1Stable64, app1Stable64Labels); err != nil {
		return err
	}

	app1StableArm := record.Record{
		AppID:    "org.gnome.Builder",
		Arch:     "aarch64",
		Branch:   "stable",
		Name:     "gnome/builder",
		Registry: "ghcr.io",
		Digest:   "sha256:1111111111111111111111111111111111111111111111111111111111111112",
	}
	app1StableArmLabels := map[string]string{
		"org.flatpak.ref":                   "app/org.gnome.Builder/aarch64/stable",
		"org.flatpak.metadata":              "[Application]\nname=org.gnome.Builder\nruntime=org.gnome.Platform/aarch64/45",
		"org.flatpak.timestamp":             "1717200000",
		"org.flatpak.installed-size":        "146800640",
		"org.flatpak.download-size":         "47185920",
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>GNOME Builder</name><summary>An IDE for writing GNOME applications</summary></component>`,
		"org.freedesktop.appstream.icon-64": "https://raw.githubusercontent.com/GNOME/gnome-builder/main/data/icons/hicolor/scalable/apps/org.gnome.Builder.svg",
	}
	if _, err := record.WriteRecord(recordsDir, app1StableArm, app1StableArmLabels); err != nil {
		return err
	}

	app1Beta64 := record.Record{
		AppID:    "org.gnome.Builder",
		Arch:     "x86_64",
		Branch:   "beta",
		Name:     "gnome/builder",
		Registry: "ghcr.io",
		Digest:   "sha256:2222222222222222222222222222222222222222222222222222222222222221",
	}
	app1Beta64Labels := map[string]string{
		"org.flatpak.ref":                   "app/org.gnome.Builder/x86_64/beta",
		"org.flatpak.metadata":              "[Application]\nname=org.gnome.Builder\nruntime=org.gnome.Platform/x86_64/46",
		"org.flatpak.timestamp":             "1717286400", // June 2, 2024
		"org.flatpak.installed-size":        "167772160",
		"org.flatpak.download-size":         "57671680",
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>GNOME Builder</name><summary>An IDE for writing GNOME applications (Beta Release)</summary></component>`,
		"org.freedesktop.appstream.icon-64": "https://raw.githubusercontent.com/GNOME/gnome-builder/main/data/icons/hicolor/scalable/apps/org.gnome.Builder.svg",
	}
	if _, err := record.WriteRecord(recordsDir, app1Beta64, app1Beta64Labels); err != nil {
		return err
	}

	if appsMode == "single" || appsMode == "1" {
		return nil
	}

	// 2. OBS Studio (stable, x86_64)
	app2Stable64 := record.Record{
		AppID:    "com.obsproject.Studio",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "obsproject/studio",
		Registry: "ghcr.io",
		Digest:   "sha256:3333333333333333333333333333333333333333333333333333333333333331",
	}
	app2Stable64Labels := map[string]string{
		"org.flatpak.ref":                   "app/com.obsproject.Studio/x86_64/stable",
		"org.flatpak.metadata":              "[Application]\nname=com.obsproject.Studio\nruntime=org.kde.Platform/x86_64/6.6",
		"org.flatpak.timestamp":             "1717113600", // May 31, 2024
		"org.flatpak.installed-size":        "83886080",
		"org.flatpak.download-size":         "26214400",
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>OBS Studio</name><summary>Free and open source software for video recording and live streaming</summary></component>`,
		"org.freedesktop.appstream.icon-64": "https://raw.githubusercontent.com/obsproject/obs-studio/master/UI/data/images/obs.png",
	}
	if _, err := record.WriteRecord(recordsDir, app2Stable64, app2Stable64Labels); err != nil {
		return err
	}

	// 3. VLC (stable, x86_64, aarch64)
	app3Stable64 := record.Record{
		AppID:    "org.videolan.VLC",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "videolan/vlc",
		Registry: "ghcr.io",
		Digest:   "sha256:4444444444444444444444444444444444444444444444444444444444444441",
	}
	app3Stable64Labels := map[string]string{
		"org.flatpak.ref":                   "app/org.videolan.VLC/x86_64/stable",
		"org.flatpak.metadata":              "[Application]\nname=org.videolan.VLC\nruntime=org.gnome.Platform/x86_64/45",
		"org.flatpak.timestamp":             "1717027200", // May 30, 2024
		"org.flatpak.installed-size":        "104857600",
		"org.flatpak.download-size":         "31457280",
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>VLC media player</name><summary>VLC is a free and open source cross-platform multimedia player and framework</summary></component>`,
		"org.freedesktop.appstream.icon-64": "https://raw.githubusercontent.com/videolan/vlc/master/share/icons/48x48/vlc.png",
	}
	if _, err := record.WriteRecord(recordsDir, app3Stable64, app3Stable64Labels); err != nil {
		return err
	}

	app3StableArm := record.Record{
		AppID:    "org.videolan.VLC",
		Arch:     "aarch64",
		Branch:   "stable",
		Name:     "videolan/vlc",
		Registry: "ghcr.io",
		Digest:   "sha256:4444444444444444444444444444444444444444444444444444444444444442",
	}
	app3StableArmLabels := map[string]string{
		"org.flatpak.ref":                   "app/org.videolan.VLC/aarch64/stable",
		"org.flatpak.metadata":              "[Application]\nname=org.videolan.VLC\nruntime=org.gnome.Platform/aarch64/45",
		"org.flatpak.timestamp":             "1717027200",
		"org.flatpak.installed-size":        "99614720",
		"org.flatpak.download-size":         "28311552",
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>VLC media player</name><summary>VLC is a free and open source cross-platform multimedia player and framework</summary></component>`,
		"org.freedesktop.appstream.icon-64": "https://raw.githubusercontent.com/videolan/vlc/master/share/icons/48x48/vlc.png",
	}
	if _, err := record.WriteRecord(recordsDir, app3StableArm, app3StableArmLabels); err != nil {
		return err
	}

	// GNOME Builder Debug (stable, x86_64)
	app1Debug64 := record.Record{
		AppID:    "org.gnome.Builder.Debug",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "gnome/builder",
		Registry: "ghcr.io",
		Digest:   "sha256:111111111111111111111111111111111111111111111111111111111111111d",
	}
	app1Debug64Labels := map[string]string{
		"org.flatpak.ref":                   "runtime/org.gnome.Builder.Debug/x86_64/stable",
		"org.flatpak.metadata":              "[Runtime]\nname=org.gnome.Builder.Debug\nparent=org.gnome.Builder/x86_64/stable",
		"org.flatpak.timestamp":             "1717200000",
		"org.flatpak.installed-size":        "314572800", // 300 MB
		"org.flatpak.download-size":         "78643200",  // 75 MB
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>GNOME Builder Debug Symbols</name><summary>Debug symbols for GNOME Builder</summary></component>`,
	}
	if _, err := record.WriteRecord(recordsDir, app1Debug64, app1Debug64Labels); err != nil {
		return err
	}

	// GNOME Builder Locale (stable, x86_64)
	app1Locale64 := record.Record{
		AppID:    "org.gnome.Builder.Locale",
		Arch:     "x86_64",
		Branch:   "stable",
		Name:     "gnome/builder",
		Registry: "ghcr.io",
		Digest:   "sha256:111111111111111111111111111111111111111111111111111111111111111l",
	}
	app1Locale64Labels := map[string]string{
		"org.flatpak.ref":                   "runtime/org.gnome.Builder.Locale/x86_64/stable",
		"org.flatpak.metadata":              "[Runtime]\nname=org.gnome.Builder.Locale\nparent=org.gnome.Builder/x86_64/stable",
		"org.flatpak.timestamp":             "1717200000",
		"org.flatpak.installed-size":        "15728640", // 15 MB
		"org.flatpak.download-size":         "2097152",  // 2 MB
		"org.freedesktop.appstream.appdata": `<?xml version="1.0" encoding="UTF-8"?><component><name>GNOME Builder Translations</name><summary>Translations for GNOME Builder</summary></component>`,
	}
	if _, err := record.WriteRecord(recordsDir, app1Locale64, app1Locale64Labels); err != nil {
		return err
	}

	return nil
}
