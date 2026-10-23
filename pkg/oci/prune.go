package oci

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/logger"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// PruneOptions configures pruning of delta tags that no longer have a target
// or that nothing references.
type PruneOptions struct {
	Registry      string
	OCIRepository string
	Insecure      bool
	OCIUsername   string
	OCIPassword   string
	DryRun        bool
}

// PruneResult reports what a prune pass found and removed.
type PruneResult struct {
	DeltaTags    int
	SourceTags   int
	IndexRebuilt bool
}

// PruneDeltas removes _delta-* tags whose target image is gone, then removes
// _deltasrc-* tags that are neither referenced by a surviving delta nor by an
// application tag. It rebuilds _deltaindex strictly from the remaining delta
// tags. Transient registry errors leave the affected tag untouched.
func PruneDeltas(opts PruneOptions) (PruneResult, error) {
	var result PruneResult

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

	tags, err := remote.List(repoRef, authOpt)
	if err != nil {
		return result, fmt.Errorf("failed to list tags: %w", err)
	}

	var deltaTags, sourceTags, appTags []string
	for _, tag := range tags {
		switch {
		case strings.HasPrefix(tag, deltaSourceTagPrefix):
			sourceTags = append(sourceTags, tag)
		case strings.HasPrefix(tag, deltaTagPrefix):
			deltaTags = append(deltaTags, tag)
		case tag == deltaIndexTag:
			// The index is rebuilt below.
		default:
			appTags = append(appTags, tag)
		}
	}

	var errs []error
	survivingDeltas := make([]string, 0, len(deltaTags))
	for _, tag := range deltaTags {
		target, err := v1.NewHash("sha256:" + strings.TrimPrefix(tag, deltaTagPrefix))
		if err != nil {
			continue
		}
		missing, err := confirmMissing(repoRef, target, authOpt)
		if err != nil {
			logger.Warn("WARNING: could not check delta target for %s; keeping it: %v", tag, err)
			survivingDeltas = append(survivingDeltas, tag)
			continue
		}
		if !missing {
			survivingDeltas = append(survivingDeltas, tag)
			continue
		}
		if opts.DryRun {
			logger.Info("[DRY RUN] Would delete delta tag %s (target %s is gone).", tag, target)
			result.DeltaTags++
			continue
		}
		if err := remote.Delete(repoRef.Tag(tag), authOpt); err != nil && !isNotFound(err) {
			errs = append(errs, fmt.Errorf("failed to delete delta tag %s: %w", tag, err))
			survivingDeltas = append(survivingDeltas, tag)
			continue
		}
		logger.Info("Deleted delta tag %s (target %s is gone).", tag, target)
		result.DeltaTags++
	}

	referencedSources, err := referencedDeltaSources(repoRef, survivingDeltas, authOpt)
	if err != nil {
		errs = append(errs, err)
	}

	appDigests := map[v1.Hash]bool{}
	if len(sourceTags) > 0 {
		for _, tag := range appTags {
			digests, err := tagManifestDigests(repoRef, tag, authOpt)
			if err != nil {
				logger.Warn("WARNING: could not resolve application tag %s; keeping source tags conservatively: %v", tag, err)
				continue
			}
			for _, digest := range digests {
				appDigests[digest] = true
			}
		}
	}

	for _, tag := range sourceTags {
		source, err := v1.NewHash("sha256:" + strings.TrimPrefix(tag, deltaSourceTagPrefix))
		if err != nil {
			continue
		}
		head, err := remote.Head(repoRef.Tag(tag), authOpt)
		if err != nil {
			if isNotFound(err) {
				// The manifest is gone; the tag is dangling regardless of
				// whether a delta still names this source.
				if opts.DryRun {
					logger.Info("[DRY RUN] Would delete dangling source tag %s.", tag)
					result.SourceTags++
					continue
				}
				if err := remote.Delete(repoRef.Tag(tag), authOpt); err != nil && !isNotFound(err) {
					errs = append(errs, fmt.Errorf("failed to delete source tag %s: %w", tag, err))
					continue
				}
				logger.Info("Deleted dangling source tag %s.", tag)
				result.SourceTags++
				continue
			}
			logger.Warn("WARNING: could not read source tag %s; keeping it: %v", tag, err)
			continue
		}
		if head.Digest != source {
			logger.Warn("WARNING: source tag %s resolves to %s, not %s; keeping it.", tag, head.Digest, source)
			continue
		}
		if referencedSources[source] || appDigests[source] {
			continue
		}
		if opts.DryRun {
			logger.Info("[DRY RUN] Would delete source tag %s (not referenced by a surviving delta).", tag)
			result.SourceTags++
			continue
		}
		if err := remote.Delete(repoRef.Tag(tag), authOpt); err != nil && !isNotFound(err) {
			errs = append(errs, fmt.Errorf("failed to delete source tag %s: %w", tag, err))
			continue
		}
		logger.Info("Deleted source tag %s (not referenced by a surviving delta).", tag)
		result.SourceTags++
	}

	if !opts.DryRun && (result.DeltaTags > 0 || result.SourceTags > 0) {
		if err := rebuildDeltaIndex(repoRef, authOpt, "", false, true); err != nil {
			errs = append(errs, fmt.Errorf("failed to rebuild delta index after pruning: %w", err))
		} else {
			result.IndexRebuilt = true
		}
	}
	return result, errors.Join(errs...)
}

// confirmMissing requires two consecutive 404s before treating an image as
// gone, so a single stale/cached negative response cannot authorise a delete.
func confirmMissing(repoRef name.Repository, digest v1.Hash, authOpt remote.Option) (bool, error) {
	for i := 0; i < 2; i++ {
		_, err := remote.Head(repoRef.Digest(digest.String()), authOpt)
		if err == nil {
			return false, nil
		}
		if !isNotFound(err) {
			return false, err
		}
	}
	return true, nil
}

func referencedDeltaSources(repoRef name.Repository, deltaTags []string, authOpt remote.Option) (map[v1.Hash]bool, error) {
	referenced := make(map[v1.Hash]bool)
	var errs []error
	for _, tag := range deltaTags {
		target, err := v1.NewHash("sha256:" + strings.TrimPrefix(tag, deltaTagPrefix))
		if err != nil {
			continue
		}
		sources, err := deltaSourceImages(repoRef, target, authOpt)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read sources for %s: %w", tag, err))
			continue
		}
		for _, source := range sources {
			referenced[source] = true
		}
	}
	return referenced, errors.Join(errs...)
}

// tagManifestDigests returns the concrete manifest digests an application tag
// points at, expanding an index to its children.
func tagManifestDigests(repoRef name.Repository, tag string, authOpt remote.Option) ([]v1.Hash, error) {
	desc, err := remote.Head(repoRef.Tag(tag), authOpt)
	if err != nil {
		return nil, err
	}
	if desc.MediaType != types.OCIImageIndex && desc.MediaType != types.DockerManifestList {
		return []v1.Hash{desc.Digest}, nil
	}
	idx, err := remote.Index(repoRef.Tag(tag), authOpt)
	if err != nil {
		return nil, err
	}
	im, err := idx.IndexManifest()
	if err != nil {
		return nil, err
	}
	out := make([]v1.Hash, 0, len(im.Manifests))
	for _, child := range im.Manifests {
		out = append(out, child.Digest)
	}
	return out, nil
}
