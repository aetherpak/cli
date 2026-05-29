package site

import (
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"strings"

	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/record"
	"github.com/aetherpak/aetherpak/pkg/signing"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

//go:embed index.html
var indexHTMLTemplate string

// SiteOptions configures the static index builder and page reconciler.
type SiteOptions struct {
	PagesURL      string
	RecordsDir    string
	SiteDir       string
	Reconcile     bool
	GPGKeys       []string // GPG private key blocks or file paths for public key export
	GPGPassphrase string
	SigDir        string // relative signature dir (defaults to "sigs")
	RemoteName    string
	RuntimeRepo   string
	RepoTitle     string
	RepoHomepage  string
	LandingPage   bool
	Insecure      bool // allow HTTP registry when reconciling
	LogoURL       string
	FaviconURL    string
	AccentColor   string
	FooterText    string
	IndexTemplate string
}

// FlatpakIndex represents the JSON model of the Flatpak index/static.
type FlatpakIndex struct {
	Registry string               `json:"Registry"`
	Results  []IndexResultPackage `json:"Results"`
}

type IndexResultPackage struct {
	Name   string       `json:"Name"` // OCI repository name (e.g. owner/repo)
	Images []IndexImage `json:"Images"`
}

type IndexImage struct {
	Digest       string            `json:"Digest"`
	MediaType    string            `json:"MediaType"`
	OS           string            `json:"OS"`
	Architecture string            `json:"Architecture"`
	Tags         []string          `json:"Tags"`
	Labels       map[string]string `json:"Labels"`
}

// BuildSite fetches the old index, merges new cell records, reconciles, and generates output files.
func BuildSite(opts SiteOptions) error {
	logger.Info("Aggregating deployment files under site directory: %s", opts.SiteDir)

	if err := os.MkdirAll(opts.SiteDir, 0755); err != nil {
		return fmt.Errorf("failed to create site directory: %w", err)
	}

	sigDirName := opts.SigDir
	if sigDirName == "" {
		sigDirName = "sigs"
	}

	// 1. Export GPG public key material if keys are supplied
	var gpgKeyBase64 string
	var fingerprint string
	var filteredKeys []string
	for _, k := range opts.GPGKeys {
		if k != "" {
			filteredKeys = append(filteredKeys, k)
		}
	}

	if len(filteredKeys) > 0 {
		signer, err := signing.NewSigner(filteredKeys, opts.GPGPassphrase)
		if err != nil {
			return fmt.Errorf("failed to load GPG keys for public export: %w", err)
		}
		fingerprint = signer.Fingerprint()

		// Export armored keyring to key.asc
		armoredKey, err := signer.ExportArmoredPublicKeyRing()
		if err != nil {
			return fmt.Errorf("failed to export armored public keyring: %w", err)
		}

		sigDir := filepath.Join(opts.SiteDir, sigDirName)
		if err := os.MkdirAll(sigDir, 0755); err != nil {
			return fmt.Errorf("failed to create sigs directory: %w", err)
		}

		keyPath := filepath.Join(sigDir, "key.asc")
		if err := os.WriteFile(keyPath, []byte(armoredKey), 0644); err != nil {
			return fmt.Errorf("failed to write key.asc: %w", err)
		}
		logger.Info("Exported armored GPG public keyring to: %s", keyPath)

		// Export base64 binary keyring
		b64Key, err := signer.ExportBase64PublicKeyRing()
		if err != nil {
			return fmt.Errorf("failed to export base64 public keyring: %w", err)
		}
		gpgKeyBase64 = b64Key
	}

	// 2. Fetch active production index from Pages URL if available
	var index FlatpakIndex
	index.Results = []IndexResultPackage{}

	if opts.PagesURL != "" {
		url := strings.TrimSuffix(opts.PagesURL, "/") + "/index/static"
		logger.Info("Fetching active production index from Pages: %s", url)

		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var fetched FlatpakIndex
			if err := json.NewDecoder(resp.Body).Decode(&fetched); err == nil {
				index = fetched
				logger.Info("Successfully seeded index from active Pages site.")
			}
		} else {
			logger.Debug("No active static index fetched or status is not OK.")
		}
	}

	// 3. Iterate and merge local record.json cells, and copy signatures
	records, err := record.IterRecords(opts.RecordsDir)
	if err != nil {
		return fmt.Errorf("failed to load records: %w", err)
	}

	logger.Info("Found %d execution records to merge.", len(records))
	for _, recWLabels := range records {
		rec := recWLabels.Record
		labels := recWLabels.Labels

		// Resolve index registry
		if index.Registry == "" {
			index.Registry = rec.Registry
		}

		// Merge logic (equivalent to merge_index.py)
		logger.Debug("Merging cell record for app: %s (%s)", rec.AppID, rec.Arch)
		mergeRecord(&index, rec, labels)

		// Copy cell's sigs directory into _site/sigs
		cellDir, err := rec.CellDir(opts.RecordsDir)
		if err == nil {
			cellSigs := filepath.Join(cellDir, "sigs")
			if info, err := os.Stat(cellSigs); err == nil && info.IsDir() {
				siteSigs := filepath.Join(opts.SiteDir, sigDirName)
				err := filepath.Walk(cellSigs, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if info.IsDir() {
						return nil
					}
					relPath, err := filepath.Rel(cellSigs, path)
					if err != nil {
						return err
					}
					dstPath := filepath.Join(siteSigs, relPath)
					if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
						return err
					}
					return copyFile(path, dstPath)
				})
				if err != nil {
					logger.Error("Failed to copy signatures for %s: %v", rec.AppID, err)
				} else {
					logger.Debug("Signatures copied for cell: %s", rec.AppID)
				}
			}
		}
	}

	// 4. Reconcile missing OCI images (reconcile.py)
	if opts.Reconcile && len(index.Results) > 0 {
		logger.Info("Reconciling index: validating OCI repository images.")
		var reconciledResults []IndexResultPackage

		for _, pkg := range index.Results {
			var reconciledImages []IndexImage

			for _, img := range pkg.Images {
				exists, err := checkDigestExists(index.Registry, pkg.Name, img.Digest, opts.Insecure)
				if err != nil {
					logger.Debug("Error checking digest existence: %v", err)
					exists = true // Keep on error
				}

				if exists {
					reconciledImages = append(reconciledImages, img)
				} else {
					logger.Info("Pruning missing OCI image from index: %s (digest: %s)", pkg.Name, img.Digest)
				}
			}

			if len(reconciledImages) > 0 {
				pkg.Images = reconciledImages
				reconciledResults = append(reconciledResults, pkg)
			}
		}
		index.Results = reconciledResults
	}

	// 5. Backfill GPG signatures from Pages URL
	backfillSignatures(opts, index, sigDirName)

	// 6. Generate deployment directories and output files
	if err := writeIndexFile(opts.SiteDir, index); err != nil {
		return err
	}

	if err := writeFlatpakRepoFile(opts.SiteDir, index.Registry, gpgKeyBase64, opts); err != nil {
		return err
	}

	var sigLookasideURL string
	if gpgKeyBase64 != "" && opts.PagesURL != "" {
		sigLookasideURL = strings.TrimSuffix(opts.PagesURL, "/") + "/" + sigDirName
	}

	if err := writeFlatpakRefs(opts.SiteDir, index, gpgKeyBase64, sigLookasideURL, opts); err != nil {
		return err
	}

	// 7. Write signing.json GPG manifest
	if err := writeSigningJSON(opts.SiteDir, sigDirName, fingerprint, opts); err != nil {
		return err
	}

	// 8. Generate index.html landing page
	if opts.LandingPage {
		remote := opts.RemoteName
		if remote == "" {
			remote = "aetherpak"
		}
		title := opts.RepoTitle
		if title == "" {
			title = "Flatpak Repository"
		}

		accent := opts.AccentColor
		if accent == "" {
			accent = "#8b5cf6"
		}

		logoHTML := ""
		if opts.LogoURL != "" {
			logoHTML = fmt.Sprintf(`<img src="%s" alt="Logo" style="max-height: 64px; margin-bottom: 1rem; border-radius: 8px;">`, opts.LogoURL)
		}

		footerText := opts.FooterText
		if footerText == "" {
			footerText = `Powered by <a href="https://aetherpak.org/" target="_blank" rel="noopener">AetherPak</a>`
		}

		html := indexHTMLTemplate
		if opts.IndexTemplate != "" {
			data, err := os.ReadFile(opts.IndexTemplate)
			if err != nil {
				return fmt.Errorf("failed to read custom index template %q: %w", opts.IndexTemplate, err)
			}
			html = string(data)
		}
		html = strings.ReplaceAll(html, "__AETHERPAK_REMOTE_NAME__", remote)
		html = strings.ReplaceAll(html, "__AETHERPAK_REPO_TITLE__", title)
		html = strings.ReplaceAll(html, "__AETHERPAK_BRANDING_ACCENT_COLOR__", accent)
		html = strings.ReplaceAll(html, "__AETHERPAK_BRANDING_FAVICON_URL__", opts.FaviconURL)
		html = strings.ReplaceAll(html, "__AETHERPAK_BRANDING_LOGO_HTML__", logoHTML)
		html = strings.ReplaceAll(html, "__AETHERPAK_BRANDING_FOOTER_TEXT__", footerText)

		indexPath := filepath.Join(opts.SiteDir, "index.html")
		if err := os.WriteFile(indexPath, []byte(html), 0644); err != nil {
			return fmt.Errorf("failed to write landing page index.html: %w", err)
		}
		logger.Debug("Landing page written to: %s", indexPath)
	}

	logger.Info("Site aggregation completed successfully.")
	return nil
}

func checkDigestExists(registry, repository, digest string, insecure bool) (bool, error) {
	regClean := registry
	if idx := strings.Index(regClean, "://"); idx != -1 {
		regClean = regClean[idx+3:]
	}

	repoName := regClean + "/" + repository
	var nameOpts []name.Option
	if insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}
	repoRef, err := name.NewRepository(repoName, nameOpts...)
	if err != nil {
		return false, err
	}

	digestRef := repoRef.Digest(digest)

	var authOpt remote.Option
	username := os.Getenv("OCI_USERNAME")
	password := os.Getenv("OCI_PASSWORD")
	if username != "" && password != "" {
		authOpt = remote.WithAuth(&authn.Basic{
			Username: username,
			Password: password,
		})
	} else {
		authOpt = remote.WithAuthFromKeychain(authn.DefaultKeychain)
	}

	_, err = remote.Head(digestRef, authOpt)
	if err != nil {
		if tErr, ok := err.(*transport.Error); ok {
			if tErr.StatusCode == http.StatusNotFound {
				// Definitive 404 Not Found only!
				return false, nil
			}
		}
		logger.Debug("Registry HEAD error for %s (treating as exists): %v", digestRef.Name(), err)
		return true, nil
	}

	return true, nil
}

func mapArch(flatpakArch string) string {
	switch strings.ToLower(flatpakArch) {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	case "i386", "i586", "i686":
		return "386"
	case "arm", "armv7hl":
		return "arm"
	default:
		return strings.ToLower(flatpakArch)
	}
}

func mergeRecord(index *FlatpakIndex, rec record.Record, labels map[string]string) {
	// Find or create repository result package
	var packageIndex = -1
	for i, pkg := range index.Results {
		if pkg.Name == rec.Name {
			packageIndex = i
			break
		}
	}

	if packageIndex == -1 {
		index.Results = append(index.Results, IndexResultPackage{
			Name:   rec.Name,
			Images: []IndexImage{},
		})
		packageIndex = len(index.Results) - 1
	}

	pkg := &index.Results[packageIndex]

	ociArch := mapArch(rec.Arch)

	// Find or update image entry for target ref+arch
	targetRef := fmt.Sprintf("app/%s/%s/%s", rec.AppID, rec.Arch, rec.Branch)
	var imageIndex = -1
	for i, img := range pkg.Images {
		if img.Labels["org.flatpak.ref"] == targetRef && img.Architecture == ociArch {
			imageIndex = i
			break
		}
	}

	newImage := IndexImage{
		Digest:       rec.Digest,
		MediaType:    "application/vnd.oci.image.manifest.v1+json",
		OS:           "linux",
		Architecture: ociArch,
		Tags:         []string{rec.Branch},
		Labels:       labels,
	}

	if imageIndex == -1 {
		pkg.Images = append(pkg.Images, newImage)
	} else {
		pkg.Images[imageIndex] = newImage
	}

	// Clean up duplicate tag entries and sort images deterministically
	for i := range pkg.Images {
		sort.Strings(pkg.Images[i].Tags)
	}
}

func writeIndexFile(siteDir string, index FlatpakIndex) error {
	indexDir := filepath.Join(siteDir, "index")
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		return fmt.Errorf("failed to create index directory: %w", err)
	}

	indexPath := filepath.Join(indexDir, "static")
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize static index: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write index/static file: %w", err)
	}

	logger.Debug("Index static asset written: %s", indexPath)
	return nil
}

func writeFlatpakRepoFile(siteDir string, registry string, gpgKeyBase64 string, opts SiteOptions) error {
	remote := opts.RemoteName
	if remote == "" {
		remote = "aetherpak"
	}
	repoPath := filepath.Join(siteDir, remote+".flatpakrepo")

	title := opts.RepoTitle
	if title == "" {
		title = "Flatpak Repository"
	}

	homepage := opts.RepoHomepage
	if homepage == "" {
		if opts.PagesURL != "" {
			homepage = opts.PagesURL
		} else {
			homepage = "https://github.com/aetherpak/aetherpak"
		}
	}

	content := fmt.Sprintf(`[Flatpak Repo]
Title=%s
Url=oci+%s
Homepage=%s
Comment=Flatpak repository powered by AetherPak (Pages index + OCI registry blobs)
`, title, opts.PagesURL, homepage)

	if gpgKeyBase64 != "" {
		content += fmt.Sprintf("GPGKey=%s\n", gpgKeyBase64)
	}

	if err := os.WriteFile(repoPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write flatpakrepo file: %w", err)
	}

	logger.Debug("Flatpakrepo reference created: %s", repoPath)
	return nil
}

func writeFlatpakRefs(siteDir string, index FlatpakIndex, gpgKeyBase64 string, sigLookasideURL string, opts SiteOptions) error {
	refsDir := filepath.Join(siteDir, "refs")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		return fmt.Errorf("failed to create refs directory: %w", err)
	}

	remoteName := opts.RemoteName
	if remoteName == "" {
		remoteName = "aetherpak"
	}

	for _, pkg := range index.Results {
		for _, img := range pkg.Images {
			// Note: This loops per image, writing a flatpakref for each. If there are multiple arches (images)
			// of the same (app, branch), it will repeatedly write to the same file. This is harmless but redundant (last-write-wins).
			refVal := img.Labels["org.flatpak.ref"]
			parts := strings.Split(refVal, "/")
			if len(parts) < 4 {
				continue
			}
			appID := parts[1]
			branch := parts[3]

			refFilename := fmt.Sprintf("%s-%s.flatpakref", appID, strings.ReplaceAll(branch, "/", "-"))
			refPath := filepath.Join(refsDir, refFilename)

			appdataXML := img.Labels["org.freedesktop.appstream.appdata"]
			title := appTitle(appdataXML, appID)

			registryURL := index.Registry
			if !strings.HasPrefix(registryURL, "http://") && !strings.HasPrefix(registryURL, "https://") {
				registryURL = "https://" + registryURL
			}
			refURL := fmt.Sprintf("oci+%s/%s", registryURL, pkg.Name)

			content := fmt.Sprintf(`[Flatpak Ref]
Title=%s
Name=%s
Branch=%s
Url=%s
IsRuntime=false
SuggestRemoteName=%s
`, title, appID, branch, refURL, remoteName)

			if opts.RuntimeRepo != "" {
				content += fmt.Sprintf("RuntimeRepo=%s\n", opts.RuntimeRepo)
			}
			if gpgKeyBase64 != "" {
				content += fmt.Sprintf("GPGKey=%s\n", gpgKeyBase64)
			}
			if sigLookasideURL != "" {
				content += fmt.Sprintf("SignatureLookaside=%s\n", sigLookasideURL)
			}

			if err := os.WriteFile(refPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write flatpakref file %s: %w", refFilename, err)
			}
			logger.Debug("Flatpakref generated: %s", refPath)
		}
	}

	return nil
}

func writeSigningJSON(siteDir string, sigDirName string, fingerprint string, opts SiteOptions) error {
	sigDir := filepath.Join(siteDir, sigDirName)
	if err := os.MkdirAll(sigDir, 0755); err != nil {
		return fmt.Errorf("failed to create sigs directory: %w", err)
	}

	manifestPath := filepath.Join(sigDir, "signing.json")
	var data map[string]interface{}

	if fingerprint != "" {
		remote := opts.RemoteName
		if remote == "" {
			remote = "aetherpak"
		}
		data = map[string]interface{}{
			"enabled":     true,
			"lookaside":   sigDirName,
			"publicKey":   fmt.Sprintf("%s/key.asc", sigDirName),
			"fingerprint": fingerprint,
			"remoteName":  remote,
		}
	} else {
		data = map[string]interface{}{
			"enabled": false,
		}
	}

	bz, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	bz = append(bz, '\n')

	if err := os.WriteFile(manifestPath, bz, 0644); err != nil {
		return fmt.Errorf("failed to write signing.json: %w", err)
	}

	logger.Debug("Signing manifest written: %s", manifestPath)
	return nil
}

func backfillSignatures(opts SiteOptions, index FlatpakIndex, sigDirName string) {
	if opts.PagesURL == "" {
		return
	}
	pagesURL := strings.TrimSuffix(opts.PagesURL, "/")

	for _, pkg := range index.Results {
		for _, img := range pkg.Images {
			// Skip stubs without metadata
			if img.Labels["org.flatpak.metadata"] == "" {
				continue
			}

			parts := strings.Split(img.Digest, ":")
			if len(parts) != 2 {
				continue
			}
			algo := parts[0]
			hexd := parts[1]

			// Try signature-1, signature-2, etc.
			for i := 1; ; i++ {
				stop := func() bool {
					relPath := fmt.Sprintf("%s/%s@%s=%s/signature-%d", sigDirName, pkg.Name, algo, hexd, i)
					localPath := filepath.Join(opts.SiteDir, relPath)

					// If it already exists, proceed to next signature index
					if _, err := os.Stat(localPath); err == nil {
						return false
					}

					url := pagesURL + "/" + relPath
					logger.Debug("Attempting to backfill signature: %s", url)

					resp, err := http.Get(url)
					if err != nil {
						logger.Debug("Failed to fetch signature %s: %v", url, err)
						return true
					}
					defer resp.Body.Close()

					if resp.StatusCode != http.StatusOK {
						// 404/other error means signature index is not present, stop sequential scan
						return true
					}

					data, err := io.ReadAll(resp.Body)
					if err != nil {
						logger.Debug("Failed to read signature body from %s: %v", url, err)
						return true
					}

					if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
						logger.Debug("Failed to create signature directory %s: %v", filepath.Dir(localPath), err)
						return true
					}

					if err := os.WriteFile(localPath, data, 0644); err != nil {
						logger.Debug("Failed to write signature file %s: %v", localPath, err)
						return true
					}

					logger.Info("Backfilled signature: %s", relPath)
					return false
				}()
				if stop {
					break
				}
			}
		}
	}
}

func appTitle(appdataXML string, appID string) string {
	fallback := appID
	if idx := strings.LastIndex(appID, "."); idx != -1 {
		fallback = appID[idx+1:]
	}
	if appdataXML == "" {
		return fallback
	}

	type Component struct {
		Names []struct {
			Lang  string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
			Value string `xml:",chardata"`
		} `xml:"name"`
	}

	decoder := xml.NewDecoder(strings.NewReader(appdataXML))
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := token.(xml.StartElement); ok {
			if se.Name.Local == "component" {
				var comp Component
				if err := decoder.DecodeElement(&comp, &se); err == nil {
					for _, name := range comp.Names {
						if name.Lang == "" {
							val := strings.TrimSpace(name.Value)
							if val != "" {
								return val
							}
						}
					}
					if len(comp.Names) > 0 {
						val := strings.TrimSpace(comp.Names[0].Value)
						if val != "" {
							return val
						}
					}
				}
			}
		}
	}
	return fallback
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
