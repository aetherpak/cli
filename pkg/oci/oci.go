package oci

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aetherpak/aetherpak/pkg/executil"
	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/aetherpak/aetherpak/pkg/record"
	"github.com/aetherpak/aetherpak/pkg/signing"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// PushOptions holds options for converting and pushing an OSTree branch to an OCI registry.
type PushOptions struct {
	AppID         string
	Arch          string
	Branch        string
	Registry      string
	OCIRepository string
	RepoPath      string
	RecordsDir    string
	GPGKeys       []string // GPG private key blocks or file paths
	GPGPassphrase []byte   // unlocks passphrase-protected keys
	Insecure      bool
	Executor      executil.Executor
	OCIUsername   string
	OCIPassword   string
	NoSign        bool
	AllowUnsigned bool
	RefType       string // "app" or "runtime"; defaults to "app"
}

// PushResult reports the coordinates of a completed push for CI consumption.
type PushResult struct {
	Digest  string
	Tag     string
	CellDir string
}

// Push converts the OSTree repo branch to OCI, signs it with each GPGKey, pushes to registry,
// and writes the parallel execution record packages.
func Push(opts PushOptions) (PushResult, error) {
	if opts.Executor == nil {
		opts.Executor = executil.NewOSExecutor()
	}
	logger.Info("Pushing application to OCI: %s/%s (arch: %s, branch: %s)", opts.Registry, opts.OCIRepository, opts.Arch, opts.Branch)

	// Format OCI tag as <app-id>-<branch>-<arch>, converting '.' to '_' in app-id to prevent
	// signature tag strip mismatch problems (see architectural invariants).
	safeAppID := strings.ReplaceAll(opts.AppID, ".", "_")
	tag := fmt.Sprintf("%s-%s-%s", safeAppID, opts.Branch, opts.Arch)
	logger.Debug("Target OCI image tag resolved to: %s", tag)

	// 1. Compile OCI layout locally using flatpak build-bundle
	ociDir, err := os.MkdirTemp("", "aetherpak-oci-layout-*")
	if err != nil {
		return PushResult{}, fmt.Errorf("failed to create temporary OCI layout directory: %w", err)
	}
	defer os.RemoveAll(ociDir)

	logger.Info("Exporting application from OSTree to OCI bundle...")
	bundleArgs := []string{
		"build-bundle",
		"--oci",
		"--arch=" + opts.Arch,
	}
	if opts.RefType == "runtime" {
		bundleArgs = append(bundleArgs, "--runtime")
	}
	bundleArgs = append(bundleArgs,
		opts.RepoPath,
		ociDir,
		opts.AppID,
		opts.Branch,
	)
	bundleCmd := opts.Executor.Command("flatpak", bundleArgs...)
	var bundleStderr bytes.Buffer
	bundleCmd.SetStderr(&bundleStderr)
	if err := bundleCmd.Run(); err != nil {
		return PushResult{}, fmt.Errorf("failed to build OCI bundle layout (%w): %s", err, bundleStderr.String())
	}

	// 2. Load OCI layout directory using go-containerregistry
	logger.Debug("Loading OCI layout from: %s", ociDir)
	index, err := layout.ImageIndexFromPath(ociDir)
	if err != nil {
		return PushResult{}, fmt.Errorf("failed to load OCI image index: %w", err)
	}

	indexManifest, err := index.IndexManifest()
	if err != nil {
		return PushResult{}, fmt.Errorf("failed to read index manifest: %w", err)
	}
	if len(indexManifest.Manifests) == 0 {
		return PushResult{}, fmt.Errorf("OCI layout contains no image manifests")
	}

	// Extract details for the first image
	desc := indexManifest.Manifests[0]
	digest := desc.Digest
	logger.Info("OCI image manifest resolved, digest: %s", digest.String())

	img, err := index.Image(digest)
	if err != nil {
		return PushResult{}, fmt.Errorf("failed to load OCI image: %w", err)
	}

	// Read OCI labels to include in records
	configFile, err := img.ConfigFile()
	if err != nil {
		return PushResult{}, fmt.Errorf("failed to read OCI config: %w", err)
	}
	labels := configFile.Config.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	// Filter out empty keys
	var keys []string
	for _, k := range opts.GPGKeys {
		if k != "" {
			keys = append(keys, k)
		}
	}

	if opts.NoSign {
		if len(keys) > 0 {
			logger.Warn("GPG signing keys were provided, but signing is disabled because no-sign is enabled.")
		}
	} else {
		if len(keys) == 0 {
			if !opts.AllowUnsigned {
				return PushResult{}, fmt.Errorf("GPG signing keys are missing. To publish unsigned images, you must explicitly enable the --allow-unsigned flag or set the AETHERPAK_ALLOW_UNSIGNED environment variable")
			}
			logger.Warn("WARNING: GPG signing keys are missing. Pushing an UNSIGNED image because --allow-unsigned is enabled.")
		}
	}

	var signatures [][]byte
	if !opts.NoSign && len(keys) > 0 {
		signer, err := signing.NewSigner(keys, opts.GPGPassphrase)
		if err != nil {
			return PushResult{}, fmt.Errorf("failed to load signing GPG keys: %w", err)
		}
		// Construct the containers/image simple signing payload. The "optional"
		// object is required by the format: verifiers such as skopeo and podman
		// reject a payload without it.
		payload := struct {
			Critical struct {
				Type  string `json:"type"`
				Image struct {
					DockerManifestDigest string `json:"docker-manifest-digest"`
				} `json:"image"`
				Identity struct {
					DockerReference string `json:"docker-reference"`
				} `json:"identity"`
			} `json:"critical"`
			Optional struct {
				Creator   string `json:"creator,omitempty"`
				Timestamp int64  `json:"timestamp,omitempty"`
			} `json:"optional"`
		}{}
		payload.Critical.Type = "atomic container signature"
		payload.Critical.Image.DockerManifestDigest = digest.String()
		payload.Critical.Identity.DockerReference = fmt.Sprintf("%s/%s:%s", opts.Registry, opts.OCIRepository, tag)
		payload.Optional.Creator = "aetherpak"
		payload.Optional.Timestamp = time.Now().Unix()

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return PushResult{}, fmt.Errorf("failed to marshal simple signing payload: %w", err)
		}

		// Sign the simple signing payload bytes
		sigs, err := signer.SignWithAll(payloadBytes)
		if err != nil {
			return PushResult{}, fmt.Errorf("failed to sign manifest: %w", err)
		}
		signatures = sigs
		logger.Info("Generated %d GPG signature(s) successfully.", len(signatures))
	}

	// 4. Establish registry auth and push OCI layout index to registry
	regHost := opts.Registry
	var regOpts []name.Option
	if opts.Insecure {
		regOpts = append(regOpts, name.Insecure)
	}
	regName, err := name.NewRegistry(regHost, regOpts...)
	if err != nil {
		return PushResult{}, fmt.Errorf("invalid registry: %w", err)
	}

	repoName := opts.Registry + "/" + opts.OCIRepository
	var repoOpts []name.Option
	if opts.Insecure {
		repoOpts = append(repoOpts, name.Insecure)
	}
	repoRef, err := name.NewRepository(repoName, repoOpts...)
	if err != nil {
		return PushResult{}, fmt.Errorf("invalid repository path %q: %w", repoName, err)
	}
	targetRef := repoRef.Tag(tag)

	// Setup authentication options
	var authOpt remote.Option
	username := opts.OCIUsername
	password := opts.OCIPassword
	if username == "" {
		username = os.Getenv("OCI_USERNAME")
	}
	if password == "" {
		password = os.Getenv("OCI_PASSWORD")
	}

	if username != "" && password != "" {
		authOpt = remote.WithAuth(&authn.Basic{
			Username: username,
			Password: password,
		})
	} else {
		// Fallback to local Docker credentials config
		authOpt = remote.WithAuthFromKeychain(authn.DefaultKeychain)
	}

	logger.Info("Pushing OCI index to remote registry: %s", targetRef.Name())
	if err := remote.WriteIndex(targetRef, index, authOpt); err != nil {
		return PushResult{}, fmt.Errorf("failed to push OCI index to registry: %w", err)
	}
	logger.Info("OCI push completed successfully.")

	// 5. Write parallel records contracts to filesystem
	refType := opts.RefType
	if refType == "" {
		refType = "app"
	}
	ref := fmt.Sprintf("%s/%s/%s/%s", refType, opts.AppID, opts.Arch, opts.Branch)
	scheme := "https://"
	if opts.Insecure {
		scheme = "http://"
	}
	rec := record.Record{
		AppID:    opts.AppID,
		Arch:     opts.Arch,
		Branch:   opts.Branch,
		Name:     opts.OCIRepository,
		Registry: scheme + regName.Name(), // Store canonical registry URL
		Digest:   digest.String(),
		Ref:      ref,
		Tag:      tag,
	}

	cellDir, err := record.WriteRecord(opts.RecordsDir, rec, labels)
	if err != nil {
		return PushResult{}, fmt.Errorf("failed to write execution record: %w", err)
	}
	logger.Info("Record wrote to cell: %s", cellDir)

	// If signed, write the signature blocks under the sigs/ directory in the cell
	if len(signatures) > 0 {
		digestHex := strings.TrimPrefix(digest.String(), "sha256:")
		sigsDir := filepath.Join(cellDir, "sigs", fmt.Sprintf("%s@sha256=%s", opts.OCIRepository, digestHex))
		if err := os.MkdirAll(sigsDir, 0755); err != nil {
			return PushResult{}, fmt.Errorf("failed to create sigs directory in cell: %w", err)
		}

		for idx, sigBytes := range signatures {
			sigPath := filepath.Join(sigsDir, fmt.Sprintf("signature-%d", idx+1))
			if err := os.WriteFile(sigPath, sigBytes, 0644); err != nil {
				return PushResult{}, fmt.Errorf("failed to write signature-%d file: %w", idx+1, err)
			}
			logger.Debug("Signature file created in cell: %s", sigPath)
		}
	}

	return PushResult{Digest: digest.String(), Tag: tag, CellDir: cellDir}, nil
}

// CleanTag ensures flatpak-compatible characters inside image tag names
func CleanTag(tag string) string {
	reg := regexp.MustCompile(`[^A-Za-z0-9_-]`)
	return reg.ReplaceAllString(tag, "_")
}
