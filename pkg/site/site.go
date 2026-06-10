package site

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/record"
	"github.com/aetherpak/aetherpak/pkg/signing"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"golang.org/x/sync/errgroup"
)

//go:embed index.html
var indexHTMLTemplate string

// SiteOptions configures the static index builder and page reconciler.
type SiteOptions struct {
	PagesURL            string
	RecordsDir          string
	SiteDir             string
	Reconcile           bool
	ActiveAppIDs        []string // List of active/configured application IDs. If non-empty, unconfigured apps are pruned during reconcile.
	ActiveOCIRepository string   // The active OCI repository path. If non-empty, packages with different OCI repository paths are pruned during reconcile.
	GPGKeys             []string // GPG private key blocks or file paths for public key export
	GPGPassphrase       []byte
	SigDir              string // relative signature dir (defaults to "sigs")
	RemoteName          string
	RuntimeRepo         string
	RepoTitle           string
	RepoHomepage        string
	LandingPage         bool
	Insecure            bool // allow HTTP registry when reconciling
	LogoURL             string
	FaviconURL          string
	AccentColor         string
	FooterText          string
	IndexTemplate       string
	NoSign              bool
	AllowUnsigned       bool
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

	if opts.NoSign {
		if len(filteredKeys) > 0 {
			logger.Warn("GPG signing keys were provided, but signing is disabled because no-sign is enabled.")
		}
	} else {
		if len(filteredKeys) == 0 {
			if !opts.AllowUnsigned {
				return fmt.Errorf("GPG signing keys are missing. To generate an unsigned repository, you must explicitly enable the --allow-unsigned flag or set the AETHERPAK_ALLOW_UNSIGNED environment variable")
			}
			logger.Warn("WARNING: GPG signing keys are missing. Generating an UNSIGNED repository because --allow-unsigned is enabled.")
		}
	}

	if !opts.NoSign && len(filteredKeys) > 0 {
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
		cellDir := recWLabels.Path
		if cellDir != "" {
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
			// If active OCI repository is specified, prune packages from different repositories
			if opts.ActiveOCIRepository != "" {
				if !strings.EqualFold(pkg.Name, opts.ActiveOCIRepository) {
					logger.Info("Pruning inactive OCI repository package from index: %s (active: %s)", pkg.Name, opts.ActiveOCIRepository)
					continue
				}
			}

			var reconciledImages []IndexImage

			for _, img := range pkg.Images {
				// Filter by active apps if configured
				if len(opts.ActiveAppIDs) > 0 {
					refVal := img.Labels["org.flatpak.ref"]
					parts := strings.Split(refVal, "/")
					if len(parts) >= 2 {
						appID := parts[1]
						isActive := false
						for _, activeID := range opts.ActiveAppIDs {
							if activeID == appID {
								isActive = true
								break
							}
						}
						if !isActive {
							logger.Info("Pruning inactive/unconfigured OCI app image from index: %s (app-id: %s, digest: %s)", pkg.Name, appID, img.Digest)
							continue
						}
					}
				}

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
	if err := backfillSignatures(opts, index, sigDirName); err != nil {
		return fmt.Errorf("failed to backfill signatures: %w", err)
	}

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
		htmlText := indexHTMLTemplate
		if opts.IndexTemplate != "" {
			data, err := os.ReadFile(opts.IndexTemplate)
			if err != nil {
				return fmt.Errorf("failed to read custom index template %q: %w", opts.IndexTemplate, err)
			}
			htmlText = string(data)
		}

		// Convert legacy placeholders to Go template syntax for full backward compatibility
		htmlText = strings.ReplaceAll(htmlText, "__AETHERPAK_REMOTE_NAME__", `{{.RemoteName}}`)
		htmlText = strings.ReplaceAll(htmlText, "__AETHERPAK_REPO_TITLE__", `{{.RepoTitle}}`)
		htmlText = strings.ReplaceAll(htmlText, "__AETHERPAK_BRANDING_ACCENT_COLOR__", `{{.AccentColor}}`)
		htmlText = strings.ReplaceAll(htmlText, "__AETHERPAK_BRANDING_FAVICON_URL__", `{{.FaviconURL}}`)
		htmlText = strings.ReplaceAll(htmlText, "__AETHERPAK_BRANDING_LOGO_HTML__", `{{.LogoHTML}}`)
		htmlText = strings.ReplaceAll(htmlText, "__AETHERPAK_BRANDING_FOOTER_TEXT__", `{{.FooterText}}`)

		tmpl, err := template.New("index").Funcs(template.FuncMap{
			"join":       strings.Join,
			"formatSize": formatSize,
			"formatDate": formatDate,
		}).Parse(htmlText)
		if err != nil {
			return fmt.Errorf("failed to parse landing page template: %w", err)
		}

		data := buildTemplateData(opts, index, fingerprint, gpgKeyBase64, sigDirName)

		var buf bytes.Buffer
		err = tmpl.Execute(&buf, data)
		if err != nil {
			return fmt.Errorf("failed to execute landing page template: %w", err)
		}

		indexPath := filepath.Join(opts.SiteDir, "index.html")
		if err := os.WriteFile(indexPath, buf.Bytes(), 0644); err != nil {
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
	targetRef := rec.Ref
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
`, sanitizeINIValue(title), sanitizeINIValue(opts.PagesURL), sanitizeINIValue(homepage))

	if gpgKeyBase64 != "" {
		content += fmt.Sprintf("GPGKey=%s\n", sanitizeINIValue(gpgKeyBase64))
		if opts.PagesURL != "" {
			sigLookasideURL := strings.TrimSuffix(opts.PagesURL, "/") + "/sigs"
			content += fmt.Sprintf("SignatureLookaside=%s\n", sanitizeINIValue(sigLookasideURL))
		}
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

			isRuntime := parts[0] == "runtime"
			var isRuntimeStr string
			if isRuntime {
				isRuntimeStr = "true"
			} else {
				isRuntimeStr = "false"
			}

			content := fmt.Sprintf(`[Flatpak Ref]
Title=%s
Name=%s
Branch=%s
Url=%s
IsRuntime=%s
SuggestRemoteName=%s
`, sanitizeINIValue(title), sanitizeINIValue(appID), sanitizeINIValue(branch), sanitizeINIValue(refURL), isRuntimeStr, sanitizeINIValue(remoteName))

			if opts.RuntimeRepo != "" {
				content += fmt.Sprintf("RuntimeRepo=%s\n", sanitizeINIValue(opts.RuntimeRepo))
			}
			if gpgKeyBase64 != "" {
				content += fmt.Sprintf("GPGKey=%s\n", sanitizeINIValue(gpgKeyBase64))
			}
			if sigLookasideURL != "" {
				content += fmt.Sprintf("SignatureLookaside=%s\n", sanitizeINIValue(sigLookasideURL))
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

func backfillSignatures(opts SiteOptions, index FlatpakIndex, sigDirName string) error {
	if opts.PagesURL == "" {
		return nil
	}
	pagesURL := strings.TrimSuffix(opts.PagesURL, "/")

	g := new(errgroup.Group)
	g.SetLimit(10) // Limit concurrency to 10 HTTP requests

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

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
			pkgName := pkg.Name

			g.Go(func() error {
				if algo != "sha256" && algo != "sha512" {
					logger.Warn("site: skipped backfill for digest using unsupported algorithm: %s", algo)
					return nil
				}

				isHex := true
				for _, r := range hexd {
					if !((r >= 'a' && r <= 'f') || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'F')) {
						isHex = false
						break
					}
				}
				if !isHex || len(hexd) == 0 {
					logger.Warn("site: skipped backfill for digest using non-hex string: %s", hexd)
					return nil
				}

				cleanPkg := filepath.Clean(pkgName)
				if filepath.IsAbs(cleanPkg) || strings.HasPrefix(cleanPkg, "..") || cleanPkg == ".." {
					logger.Warn("site: skipped backfill for invalid package name: %s", pkgName)
					return nil
				}

				// Try signature-1, signature-2, etc.
				for i := 1; ; i++ {
					stop, err := func() (bool, error) {
						relPath := fmt.Sprintf("%s/%s@%s=%s/signature-%d", sigDirName, cleanPkg, algo, hexd, i)
						localPath := filepath.Join(opts.SiteDir, relPath)

						under, err := isUnderDir(opts.SiteDir, localPath)
						if err != nil || !under {
							logger.Warn("site: skipped backfill of signature escaping site directory: %s", localPath)
							return true, fmt.Errorf("site: path traversal detected: %s", localPath)
						}

						// If it already exists, proceed to next signature index
						if _, err := os.Stat(localPath); err == nil {
							return false, nil
						}

						url := pagesURL + "/" + relPath
						logger.Debug("Attempting to backfill signature: %s", url)

						resp, err := client.Get(url)
						if err != nil {
							logger.Warn("Failed to fetch signature %s: %v", url, err)
							return true, nil
						}
						defer resp.Body.Close()

						if resp.StatusCode != http.StatusOK {
							if resp.StatusCode != http.StatusNotFound {
								logger.Warn("Unexpected status code %d fetching signature from %s", resp.StatusCode, url)
							}
							// 404 or other unexpected statuses mean signature is not present, stop sequential scan
							return true, nil
						}

						data, err := io.ReadAll(resp.Body)
						if err != nil {
							logger.Warn("Failed to read signature body from %s: %v", url, err)
							return true, err
						}

						if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
							logger.Warn("Failed to create signature directory %s: %v", filepath.Dir(localPath), err)
							return true, err
						}

						if err := os.WriteFile(localPath, data, 0644); err != nil {
							logger.Warn("Failed to write signature file %s: %v", localPath, err)
							return true, err
						}

						logger.Info("Backfilled signature: %s", relPath)
						return false, nil
					}()
					if err != nil {
						return err
					}
					if stop {
						break
					}
				}
				return nil
			})
		}
	}

	return g.Wait()
}

func appTitle(appdataXML string, appID string) string {
	name, _ := parseAppMetadata(appdataXML, appID)
	return name
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

func sanitizeINIValue(val string) string {
	val = strings.ReplaceAll(val, "\n", "")
	val = strings.ReplaceAll(val, "\r", "")
	return val
}

// TemplateData represents the context passed to the custom HTML landing page template.
type TemplateData struct {
	RemoteName   string
	RepoTitle    string
	PagesURL     string
	RepoHomepage string
	RuntimeRepo  string

	LogoURL     string
	LogoHTML    template.HTML
	FaviconURL  string
	AccentColor string
	FooterText  template.HTML

	Signing struct {
		Enabled     bool
		Fingerprint string
		PublicKey   string // e.g. "sigs/key.asc"
		Lookaside   string // e.g. "sigs"
	}

	Index FlatpakIndex
	Apps  []TemplateApp
}

type TemplateApp struct {
	ID       string
	Name     string
	Summary  string
	Icon     string
	Branches []TemplateBranch
}

type TemplateBranch struct {
	Branch        string
	Arches        []string
	Timestamp     int64
	FormattedDate string
	InstalledSize int64
	DownloadSize  int64
	Commit        string
	RefFile       string
	InstallCmd    string
}

type appdataComponent struct {
	XMLName xml.Name `xml:"component"`
	Names   []struct {
		Lang  string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
		Value string `xml:",chardata"`
	} `xml:"name"`
	Summaries []struct {
		Lang  string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
		Value string `xml:",chardata"`
	} `xml:"summary"`
}

func parseAppMetadata(appdataXML string, appID string) (name string, summary string) {
	fallbackName := appID
	if idx := strings.LastIndex(appID, "."); idx != -1 {
		fallbackName = appID[idx+1:]
	}
	if appdataXML == "" {
		return fallbackName, ""
	}

	var comp appdataComponent
	decoder := xml.NewDecoder(strings.NewReader(appdataXML))
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := token.(xml.StartElement); ok && se.Name.Local == "component" {
			if err := decoder.DecodeElement(&comp, &se); err == nil {
				break
			}
		}
	}

	// Pick default name (empty lang preferred, then first)
	for _, n := range comp.Names {
		if n.Lang == "" {
			name = strings.TrimSpace(n.Value)
			break
		}
	}
	if name == "" && len(comp.Names) > 0 {
		name = strings.TrimSpace(comp.Names[0].Value)
	}
	if name == "" {
		name = fallbackName
	}

	// Pick default summary
	for _, s := range comp.Summaries {
		if s.Lang == "" {
			summary = strings.TrimSpace(s.Value)
			break
		}
	}
	if summary == "" && len(comp.Summaries) > 0 {
		summary = strings.TrimSpace(comp.Summaries[0].Value)
	}

	return name, summary
}

func buildTemplateData(opts SiteOptions, index FlatpakIndex, fingerprint string, gpgKeyBase64 string, sigDirName string) TemplateData {
	data := TemplateData{
		RemoteName:   opts.RemoteName,
		RepoTitle:    opts.RepoTitle,
		PagesURL:     opts.PagesURL,
		RepoHomepage: opts.RepoHomepage,
		RuntimeRepo:  opts.RuntimeRepo,
		LogoURL:      opts.LogoURL,
		FaviconURL:   opts.FaviconURL,
		AccentColor:  opts.AccentColor,
		Index:        index,
	}

	if data.RemoteName == "" {
		data.RemoteName = "aetherpak"
	}
	if data.RepoTitle == "" {
		data.RepoTitle = "Flatpak Repository"
	}
	if data.AccentColor == "" {
		data.AccentColor = "#8b5cf6"
	}
	if opts.LogoURL != "" {
		data.LogoHTML = template.HTML(fmt.Sprintf(`<img src="%s" alt="Logo" style="max-height: 64px; margin-bottom: 1rem; border-radius: 8px;">`, html.EscapeString(opts.LogoURL)))
	}
	if opts.FooterText != "" {
		data.FooterText = template.HTML(opts.FooterText)
	} else {
		data.FooterText = template.HTML(`Powered by <a href="https://aetherpak.org/" target="_blank" rel="noopener">AetherPak</a>`)
	}

	if fingerprint != "" {
		data.Signing.Enabled = true
		data.Signing.Fingerprint = fingerprint
		data.Signing.PublicKey = fmt.Sprintf("%s/key.asc", sigDirName)
		data.Signing.Lookaside = sigDirName
	} else {
		data.Signing.Enabled = false
	}

	// Map to keep track of apps during grouping
	appMap := make(map[string]*TemplateApp)

	for _, pkg := range index.Results {
		for _, img := range pkg.Images {
			refVal := img.Labels["org.flatpak.ref"]
			if refVal == "" || img.Labels["org.flatpak.metadata"] == "" {
				continue
			}

			parts := strings.Split(refVal, "/")
			if len(parts) < 4 || (parts[0] != "app" && parts[0] != "runtime") {
				continue
			}

			appID := parts[1]
			arch := parts[2]
			branch := parts[3]

			app, exists := appMap[appID]
			if !exists {
				appName, appSummary := parseAppMetadata(img.Labels["org.freedesktop.appstream.appdata"], appID)
				iconURL := img.Labels["org.freedesktop.appstream.icon-64"]

				app = &TemplateApp{
					ID:       appID,
					Name:     appName,
					Summary:  appSummary,
					Icon:     iconURL,
					Branches: []TemplateBranch{},
				}
				appMap[appID] = app
			} else {
				// Backfill metadata if a subsequent image has richer information
				if app.Name == appID || app.Summary == "" {
					if name, summary := parseAppMetadata(img.Labels["org.freedesktop.appstream.appdata"], appID); name != appID && name != "" {
						app.Name = name
						app.Summary = summary
					}
				}
				if app.Icon == "" {
					app.Icon = img.Labels["org.freedesktop.appstream.icon-64"]
				}
			}

			// Find or create branch
			var branchIdx = -1
			for i, b := range app.Branches {
				if b.Branch == branch {
					branchIdx = i
					break
				}
			}

			var ts int64
			if tsStr := img.Labels["org.flatpak.timestamp"]; tsStr != "" {
				fmt.Sscanf(tsStr, "%d", &ts)
			}
			var isize int64
			if isizeStr := img.Labels["org.flatpak.installed-size"]; isizeStr != "" {
				fmt.Sscanf(isizeStr, "%d", &isize)
			}
			var dsize int64
			if dsizeStr := img.Labels["org.flatpak.download-size"]; dsizeStr != "" {
				fmt.Sscanf(dsizeStr, "%d", &dsize)
			}
			commit := img.Labels["org.flatpak.commit"]

			if branchIdx == -1 {
				newBranch := TemplateBranch{
					Branch:        branch,
					Arches:        []string{arch},
					Timestamp:     ts,
					InstalledSize: isize,
					DownloadSize:  dsize,
					Commit:        commit,
				}
				app.Branches = append(app.Branches, newBranch)
			} else {
				b := &app.Branches[branchIdx]
				// Add arch if unique
				foundArch := false
				for _, a := range b.Arches {
					if a == arch {
						foundArch = true
						break
					}
				}
				if !foundArch {
					b.Arches = append(b.Arches, arch)
				}
				if ts > b.Timestamp {
					b.Timestamp = ts
					b.InstalledSize = isize
					b.DownloadSize = dsize
					b.Commit = commit
				}
			}
		}
	}

	// Convert map to slice
	apps := make([]TemplateApp, 0, len(appMap))
	for _, app := range appMap {
		// Postprocess each app's branches
		for i := range app.Branches {
			b := &app.Branches[i]
			sort.Strings(b.Arches)
			if b.Timestamp > 0 {
				t := time.Unix(b.Timestamp, 0).UTC()
				b.FormattedDate = t.Format("Jan 02, 2006")
			}
			b.RefFile = fmt.Sprintf("refs/%s-%s.flatpakref", app.ID, strings.ReplaceAll(b.Branch, "/", "-"))
			b.InstallCmd = fmt.Sprintf("flatpak install --user %s %s//%s", data.RemoteName, app.ID, b.Branch)
		}

		// Sort branches by timestamp descending
		sort.Slice(app.Branches, func(i, j int) bool {
			return app.Branches[i].Timestamp > app.Branches[j].Timestamp
		})

		apps = append(apps, *app)
	}

	// Sort apps by ID (alphabetically)
	sort.Slice(apps, func(i, j int) bool {
		return apps[i].ID < apps[j].ID
	})

	data.Apps = apps
	return data
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	val := float64(bytes) / float64(div)
	if val < 10 {
		return fmt.Sprintf("%.1f %s", val, units[exp])
	}
	return fmt.Sprintf("%.0f %s", val, units[exp])
}

func formatDate(timestamp int64, layout string) string {
	if timestamp == 0 {
		return ""
	}
	return time.Unix(timestamp, 0).UTC().Format(layout)
}

func isUnderDir(dir, path string) (bool, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false, err
	}
	if strings.HasPrefix(rel, "..") || rel == ".." {
		return false, nil
	}
	return true, nil
}
