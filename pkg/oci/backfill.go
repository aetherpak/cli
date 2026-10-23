package oci

import (
	"fmt"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// BackfillOptions configures a delta backfill or repair from explicit image
// manifest digests.
type BackfillOptions struct {
	Registry      string
	OCIRepository string
	Insecure      bool
	OCIUsername   string
	OCIPassword   string
	ToDigest      string
	FromDigests   []string
	WorkDir       string
	DryRun        bool
}

// BackfillResult reports what a backfill pass generated.
type BackfillResult struct {
	TargetDigest  string
	Layers        int
	SmallestBytes int64
	Published     bool
}

// BackfillDeltas generates and publishes deltas for an existing target image
// from one or more existing source images. It does not push the images again
// and is meant to repair a missing or lost delta manifest.
func BackfillDeltas(opts BackfillOptions) (BackfillResult, error) {
	var result BackfillResult
	if opts.ToDigest == "" {
		return result, fmt.Errorf("to digest is required")
	}
	if len(opts.FromDigests) == 0 {
		return result, fmt.Errorf("at least one from digest is required")
	}

	var repoOpts []name.Option
	if opts.Insecure {
		repoOpts = append(repoOpts, name.Insecure)
	}
	repoName := opts.Registry + "/" + opts.OCIRepository
	repoRef, err := name.NewRepository(repoName, repoOpts...)
	if err != nil {
		return result, fmt.Errorf("invalid repository path %q: %w", repoName, err)
	}
	authOpt := resolveAuth(opts.OCIUsername, opts.OCIPassword)

	toHash, err := parseDigest(opts.ToDigest)
	if err != nil {
		return result, fmt.Errorf("invalid to digest: %w", err)
	}
	result.TargetDigest = toHash.String()
	toRef := repoRef.Digest(toHash.String())
	newImg, err := remote.Image(toRef, authOpt)
	if err != nil {
		return result, fmt.Errorf("failed to read target image %s: %w", toHash, err)
	}
	cfg, err := newImg.ConfigFile()
	if err != nil {
		return result, fmt.Errorf("failed to read target image config: %w", err)
	}

	sources := make([]deltaSource, 0, len(opts.FromDigests))
	seen := map[v1.Hash]bool{}
	for _, value := range opts.FromDigests {
		fromHash, err := parseDigest(value)
		if err != nil {
			return result, fmt.Errorf("invalid from digest %q: %w", value, err)
		}
		if fromHash == toHash || seen[fromHash] {
			continue
		}
		seen[fromHash] = true
		sources = append(sources, deltaSource{
			ref:  repoRef.Digest(fromHash.String()),
			desc: &v1.Descriptor{MediaType: types.OCIManifestSchema1, Digest: fromHash},
		})
	}
	if len(sources) == 0 {
		return result, fmt.Errorf("no usable from digests remain after de-duplication")
	}

	delta, err := prepareImageDeltas(repoRef, sources, cfg.Architecture, newImg, toHash, authOpt, opts.WorkDir, !opts.DryRun)
	if err != nil {
		return result, err
	}
	if delta == nil {
		logger.Info("No smaller delta was generated for %s.", toHash)
		return result, nil
	}
	defer delta.Close()

	result.Layers = len(delta.layers)
	if len(delta.layers) > 0 {
		result.SmallestBytes = delta.layers[0].size
	}
	if opts.DryRun {
		logger.Info("[DRY RUN] Would publish %d delta layer(s) for %s.", result.Layers, toHash)
		return result, nil
	}
	if err := publishImageDelta(repoRef, authOpt, delta); err != nil {
		return result, err
	}
	result.Published = true
	return result, nil
}

func parseDigest(value string) (v1.Hash, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return v1.Hash{}, fmt.Errorf("empty digest")
	}
	if !strings.Contains(value, ":") {
		value = "sha256:" + value
	}
	return v1.NewHash(value)
}
