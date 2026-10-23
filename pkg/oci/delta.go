package oci

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aetherpak/aetherpak/pkg/logger"
	tardiff "github.com/containers/tar-diff/pkg/tar-diff"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

const (
	// deltaIndexTag is the mutable tag Flatpak reads to discover delta manifests.
	deltaIndexTag = "_deltaindex"
	// deltaTagPrefix marks the per-target tags that retain delta manifests and
	// make the index reconstructable from the tag set.
	deltaTagPrefix = "_delta-"

	deltaLayerMediaType  = types.MediaType("application/vnd.tar-diff")
	deltaConfigMediaType = types.MediaType("application/vnd.redhat.delta.config.v1+json")

	deltaTargetAnnotation    = "io.github.containers.delta.target"
	deltaFromAnnotation      = "io.github.containers.delta.from"
	deltaToAnnotation        = "io.github.containers.delta.to"
	deltaFromImageAnnotation = "io.aetherpak.delta.from-image"
	deltaSourcesAnnotation   = "io.aetherpak.delta.sources"
	deltaSourceTagPrefix     = "_deltasrc-"

	// deltaIndexRetries bounds the read-merge-write loop on the shared _deltaindex tag.
	deltaIndexRetries = 6
	// deltaMaxLayerSize keeps one release from staging multi-gigabyte layers in
	// the workspace. Flatpak application layers are normally far below this.
	deltaMaxLayerSize = int64(2 << 30)
	deltaMaxWorkBytes = int64(4 << 30)
	maxDeltaBack      = 10
)

// deltaConfig is the empty configuration object required by the delta image
// manifest specification.
var deltaConfig = []byte("{}")

// deltaLayer is one generated tar-diff layer waiting to be uploaded.
type deltaLayer struct {
	path       string
	digest     v1.Hash
	size       int64
	descriptor v1.Descriptor
}

// imageDelta holds the local artifacts needed to publish one delta after the
// target image has been pushed.
type imageDelta struct {
	dir            string
	layers         []deltaLayer
	layerSize      int64
	manifestBytes  []byte
	manifestDigest v1.Hash
	manifestSize   int64
	targetDigest   v1.Hash
}

// deltaSource is a previous image used as a delta source.
type deltaSource struct {
	ref  name.Reference
	desc *v1.Descriptor
}

// Close removes the temporary files backing the delta.
func (d *imageDelta) Close() {
	if d != nil && d.dir != "" {
		_ = os.RemoveAll(d.dir)
	}
}

// resolveDeltaSource captures the image currently at tag before the tag is
// overwritten. It resolves through an image index to the concrete manifest so
// the later fetch does not depend on an untagged index surviving registry GC.
func resolveDeltaSource(repoRef name.Repository, tag, arch string, authOpt remote.Option) (name.Reference, *v1.Descriptor, error) {
	tagRef := repoRef.Tag(tag)
	desc, err := remote.Head(tagRef, authOpt)
	if err != nil {
		if isNotFound(err) {
			logger.Info("No previous image at %s; skipping image delta.", tagRef.Name())
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to resolve previous image %s: %w", tagRef.Name(), err)
	}

	if desc.MediaType == types.OCIImageIndex || desc.MediaType == types.DockerManifestList {
		idx, err := remote.Index(tagRef, authOpt)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load previous image index: %w", err)
		}
		im, err := idx.IndexManifest()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read previous image index: %w", err)
		}
		chosen, err := chooseImageManifest(im.Manifests, arch)
		if err != nil {
			return nil, nil, err
		}
		child := chosen
		if child.MediaType == "" {
			child.MediaType = types.OCIManifestSchema1
		}
		return repoRef.Digest(chosen.Digest.String()), &child, nil
	}

	if desc.Digest == (v1.Hash{}) {
		return tagRef, desc, nil
	}
	return repoRef.Digest(desc.Digest.String()), desc, nil
}

func chooseImageManifest(manifests []v1.Descriptor, arch string) (v1.Descriptor, error) {
	if len(manifests) == 0 {
		return v1.Descriptor{}, fmt.Errorf("previous image index is empty")
	}
	chosen := manifests[0]
	if len(manifests) == 1 {
		return chosen, nil
	}
	for _, candidate := range manifests {
		if candidate.Platform != nil && candidate.Platform.Architecture == arch {
			return candidate, nil
		}
	}
	return v1.Descriptor{}, fmt.Errorf("previous image index has no manifest for architecture %q", arch)
}

// resolveDeltaSources returns the immediate previous image plus older sources
// reachable through the delta manifests published for it. delta_back limits
// the total number of sources.
func resolveDeltaSources(repoRef name.Repository, tag, arch, flatpakRef string, back int, authOpt remote.Option) ([]deltaSource, error) {
	ref, desc, err := resolveDeltaSource(repoRef, tag, arch, authOpt)
	if err != nil || desc == nil {
		return nil, err
	}
	if back < 1 {
		back = 1
	}
	sources := []deltaSource{{ref: ref, desc: desc}}
	if back == 1 {
		return sources, nil
	}

	seen := map[v1.Hash]bool{desc.Digest: true}
	queue := []v1.Hash{desc.Digest}
	for len(sources) < back && len(queue) > 0 {
		target := queue[0]
		queue = queue[1:]
		fromImages, err := deltaSourceImages(repoRef, target, authOpt)
		if err != nil {
			logger.Warn("WARNING: could not read earlier delta sources for %s: %v", target, err)
			continue
		}
		if len(fromImages) == 0 {
			logger.Info("No older delta sources are recorded for %s.", target)
		}
		for _, sourceDigest := range fromImages {
			if seen[sourceDigest] {
				continue
			}
			seen[sourceDigest] = true
			sourceRef := repoRef.Digest(sourceDigest.String())
			img, err := remote.Image(sourceRef, authOpt)
			if err != nil {
				logger.Warn("WARNING: earlier delta source %s is unavailable: %v", sourceDigest, err)
				continue
			}
			cfg, err := img.ConfigFile()
			if err != nil {
				logger.Warn("WARNING: could not read earlier delta source %s: %v", sourceDigest, err)
				continue
			}
			if cfg.Architecture != "" && cfg.Architecture != arch {
				continue
			}
			if got := cfg.Config.Labels["org.flatpak.ref"]; got != "" && got != flatpakRef {
				continue
			}
			sources = append(sources, deltaSource{
				ref:  sourceRef,
				desc: &v1.Descriptor{MediaType: types.OCIManifestSchema1, Digest: sourceDigest},
			})
			if len(sources) >= back {
				break
			}
			queue = append(queue, sourceDigest)
		}
	}
	return sources, nil
}

// deltaSourceImages reads the source image digests recorded in the delta
// manifest for target. Older manifests without the annotation stop the chain.
func deltaSourceImages(repoRef name.Repository, target v1.Hash, authOpt remote.Option) ([]v1.Hash, error) {
	ref := repoRef.Tag(deltaManifestTag(target))
	desc, err := remote.Get(ref, authOpt)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read delta manifest for %s: %w", target, err)
	}
	var manifest v1.Manifest
	if err := json.Unmarshal(desc.Manifest, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse delta manifest for %s: %w", target, err)
	}
	if ordered := manifest.Annotations[deltaSourcesAnnotation]; ordered != "" {
		var values []string
		if err := json.Unmarshal([]byte(ordered), &values); err == nil {
			out := make([]v1.Hash, 0, len(values))
			for _, value := range values {
				h, err := v1.NewHash(value)
				if err != nil || containsHash(out, h) {
					continue
				}
				out = append(out, h)
			}
			return out, nil
		}
	}

	// Fall back to layer annotations for manifests written before the ordered
	// source list was recorded.
	var out []v1.Hash
	for _, layer := range manifest.Layers {
		value := layer.Annotations[deltaFromImageAnnotation]
		if value == "" {
			continue
		}
		h, err := v1.NewHash(value)
		if err != nil {
			continue
		}
		if !containsHash(out, h) {
			out = append(out, h)
		}
	}
	return out, nil
}

func containsHash(values []v1.Hash, want v1.Hash) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// prepareImageDeltas generates one delta layer per source. It returns nil when
// no delta is smaller than the full layer.
func prepareImageDeltas(repoRef name.Repository, sources []deltaSource, arch string, newImg v1.Image, newDigest v1.Hash, authOpt remote.Option, workDir string) (*imageDelta, error) {
	newCfg, err := newImg.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to read target image config: %w", err)
	}
	// Flatpak records one xa.diff-id for the deployment and uses it as the
	// delta source for every layer. Its OCI images are single-layer, so refuse
	// to guess for multi-layer images.
	if len(newCfg.RootFS.DiffIDs) != 1 {
		logger.Info("Target image has %d layers; image deltas currently support single-layer images only. Recording the source chain without a usable delta.", len(newCfg.RootFS.DiffIDs))
		return newChainDelta(sources, newDigest)
	}
	newDiffID := newCfg.RootFS.DiffIDs[0]

	newLayers, err := newImg.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to read target image layers: %w", err)
	}
	if len(newLayers) != 1 {
		logger.Info("Target image has %d layers; image deltas currently support single-layer images only. Recording the source chain without a usable delta.", len(newLayers))
		return newChainDelta(sources, newDigest)
	}
	newLayerSize, err := newLayers[0].Size()
	if err != nil {
		return nil, fmt.Errorf("failed to read target layer size: %w", err)
	}
	if newLayerSize > deltaMaxLayerSize {
		logger.Info("Target layer exceeds the %d MiB work limit; recording the source chain without a usable delta.", deltaMaxLayerSize/(1<<20))
		return newChainDelta(sources, newDigest)
	}

	workRoot, err := deltaWorkRoot(workDir)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(workRoot, ".aetherpak-delta-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create delta temp directory: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()

	newLayerPath, err := spoolLayer(newLayers[0], dir, "new")
	if err != nil {
		return nil, fmt.Errorf("failed to stage target layer: %w", err)
	}

	sourceDigests := make([]v1.Hash, len(sources))
	for i := range sources {
		sourceDigests[i] = sources[i].desc.Digest
	}

	var layers []deltaLayer
	var totalSourceBytes int64
	contentChanged := false
	seenDiffIDs := make(map[v1.Hash]bool, len(sources))
	for i, source := range sources {
		oldImg, err := imageForDelta(source.ref, source.desc, arch, authOpt)
		if err != nil {
			logger.Warn("WARNING: failed to read delta source %s: %v", source.desc.Digest, err)
			continue
		}
		if err := retainDeltaSource(repoRef, oldImg, source.desc.Digest, authOpt); err != nil {
			logger.Warn("WARNING: could not retain delta source %s: %v", source.desc.Digest, err)
		}
		oldCfg, err := oldImg.ConfigFile()
		if err != nil {
			logger.Warn("WARNING: failed to read delta source config %s: %v", source.desc.Digest, err)
			continue
		}
		if len(oldCfg.RootFS.DiffIDs) != 1 {
			logger.Info("Delta source %s has %d layers; skipping it.", source.desc.Digest, len(oldCfg.RootFS.DiffIDs))
			continue
		}
		oldDiffID := oldCfg.RootFS.DiffIDs[0]
		if oldDiffID == newDiffID {
			continue
		}
		contentChanged = true
		if seenDiffIDs[oldDiffID] {
			continue
		}
		seenDiffIDs[oldDiffID] = true

		oldLayers, err := oldImg.Layers()
		if err != nil {
			logger.Warn("WARNING: failed to read delta source layers %s: %v", source.desc.Digest, err)
			continue
		}
		if len(oldLayers) != 1 {
			logger.Info("Delta source %s has %d layers; skipping it.", source.desc.Digest, len(oldLayers))
			continue
		}
		oldLayerSize, err := oldLayers[0].Size()
		if err != nil {
			logger.Warn("WARNING: failed to read delta source layer size %s: %v", source.desc.Digest, err)
			continue
		}
		if oldLayerSize > deltaMaxLayerSize {
			logger.Info("Skipping delta source %s: layer exceeds the %d MiB work limit.", source.desc.Digest, deltaMaxLayerSize/(1<<20))
			continue
		}
		if totalSourceBytes+oldLayerSize > deltaMaxWorkBytes {
			logger.Info("Skipping delta source %s: total source budget exceeded.", source.desc.Digest)
			continue
		}
		totalSourceBytes += oldLayerSize

		oldLayerPath, err := spoolLayer(oldLayers[0], dir, fmt.Sprintf("old-%d", i))
		if err != nil {
			logger.Warn("WARNING: failed to stage delta source %s: %v", source.desc.Digest, err)
			continue
		}
		deltaPath := filepath.Join(dir, fmt.Sprintf("delta-%d.tardiff", i))
		deltaSize, err := generateDeltaFile(oldLayerPath, newLayerPath, deltaPath)
		_ = os.Remove(oldLayerPath)
		if err != nil {
			_ = os.Remove(deltaPath)
			logger.Warn("WARNING: failed to generate delta from %s: %v", source.desc.Digest, err)
			continue
		}
		if deltaSize >= newLayerSize {
			logger.Info("Delta from %s is not smaller than the full layer; skipping.", source.desc.Digest)
			_ = os.Remove(deltaPath)
			continue
		}
		deltaDigest, _, err := hashFile(deltaPath)
		if err != nil {
			_ = os.Remove(deltaPath)
			logger.Warn("WARNING: failed to hash delta from %s: %v", source.desc.Digest, err)
			continue
		}
		descriptor := v1.Descriptor{
			MediaType: deltaLayerMediaType,
			Size:      deltaSize,
			Digest:    deltaDigest,
			Annotations: map[string]string{
				deltaFromAnnotation:      oldDiffID.String(),
				deltaToAnnotation:        newDiffID.String(),
				deltaFromImageAnnotation: source.desc.Digest.String(),
			},
		}
		layers = append(layers, deltaLayer{
			path:       deltaPath,
			digest:     deltaDigest,
			size:       deltaSize,
			descriptor: descriptor,
		})
		logger.Info("Generated delta layer from %s: %d bytes.", source.desc.Digest, deltaSize)
	}

	if len(layers) == 0 {
		if !contentChanged {
			return nil, nil
		}
		logger.Info("No source produced a smaller delta; recording the source chain without a usable delta.")
		return newChainDelta(sources, newDigest)
	}
	// Flatpak picks the first matching from/to layer, so keep the smallest
	// delta first for clients that do not compare sizes.
	sort.Slice(layers, func(i, j int) bool { return layers[i].size < layers[j].size })

	descriptors := make([]v1.Descriptor, len(layers))
	for i := range layers {
		descriptors[i] = layers[i].descriptor
	}
	manifestBytes, manifestDigest, manifestSize, err := buildDeltaManifest(descriptors, sourceDigests, newDigest)
	if err != nil {
		return nil, err
	}

	logger.Info("Generated %d delta layer(s) from %d source(s).", len(layers), len(sources))
	keep = true
	return &imageDelta{
		dir:            dir,
		layers:         layers,
		layerSize:      newLayerSize,
		manifestBytes:  manifestBytes,
		manifestDigest: manifestDigest,
		manifestSize:   manifestSize,
		targetDigest:   newDigest,
	}, nil
}

// retainDeltaSource tags the source image manifest so registries with
// aggressive untagged-manifest collection keep it for future delta chains.
func retainDeltaSource(repoRef name.Repository, img v1.Image, digest v1.Hash, authOpt remote.Option) error {
	body, err := img.RawManifest()
	if err != nil {
		return err
	}
	return putManifest(repoRef.Tag(deltaSourceTag(digest)), body, digest, int64(len(body)), authOpt)
}

func deltaSourceTag(digest v1.Hash) string {
	return deltaSourceTagPrefix + digest.Hex
}

// generateDeltaFile runs tar-diff over one source layer and the target layer.
func generateDeltaFile(oldPath, newPath, deltaPath string) (int64, error) {
	oldFile, err := os.Open(oldPath)
	if err != nil {
		return 0, err
	}
	defer oldFile.Close()
	newFile, err := os.Open(newPath)
	if err != nil {
		return 0, err
	}
	defer newFile.Close()
	deltaFile, err := os.Create(deltaPath)
	if err != nil {
		return 0, err
	}

	opts := tardiff.NewOptions()
	// Flatpak's apply implementation understands the v1 (bsdiff) format. Keep
	// the generator on that path so deltas remain consumable by current clients.
	opts.SetBinaryDiffMethod(tardiff.BinaryDiffBsdiff)
	// Keep tar-diff's analysis scratch on the same disk-backed workspace.
	opts.SetTmpDir(filepath.Dir(deltaPath))

	if err := tardiff.Diff([]io.ReadSeeker{oldFile}, newFile, deltaFile, opts); err != nil {
		_ = deltaFile.Close()
		return 0, err
	}
	if err := deltaFile.Close(); err != nil {
		return 0, err
	}
	info, err := os.Stat(deltaPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// publishImageDelta uploads the delta blob and manifest, then rebuilds the
// repository's _deltaindex from the per-target delta tags.
func publishImageDelta(repoRef name.Repository, authOpt remote.Option, d *imageDelta) error {
	if d == nil {
		return nil
	}

	// The delta manifest references an empty config object; push it so the
	// manifest is self-contained in the registry.
	if err := remote.WriteLayer(repoRef, static.NewLayer(deltaConfig, deltaConfigMediaType), authOpt); err != nil {
		return fmt.Errorf("failed to push delta config blob: %w", err)
	}

	for _, layer := range d.layers {
		blob := &fileLayer{
			path:      layer.path,
			digest:    layer.digest,
			size:      layer.size,
			mediaType: deltaLayerMediaType,
		}
		if err := remote.WriteLayer(repoRef, blob, authOpt); err != nil {
			return fmt.Errorf("failed to push delta blob: %w", err)
		}
	}

	// Tagging the delta manifest keeps registries with garbage collection from
	// treating it as an orphan, and makes the tag set the source of truth for
	// rebuilding _deltaindex.
	manifestRef := repoRef.Tag(deltaManifestTag(d.targetDigest))
	if err := putManifest(manifestRef, d.manifestBytes, d.manifestDigest, d.manifestSize, authOpt); err != nil {
		return fmt.Errorf("failed to push delta manifest: %w", err)
	}

	if err := rebuildDeltaIndex(repoRef, authOpt, deltaManifestTag(d.targetDigest), true, false); err != nil {
		return err
	}

	if len(d.layers) == 0 {
		logger.Info("Published image delta chain marker: no usable delta for this release.")
		return nil
	}
	smallest := d.layers[0].size // sorted ascending by prepareImageDeltas
	pct := 0.0
	if d.layerSize > 0 {
		pct = 100 * float64(smallest) / float64(d.layerSize)
	}
	logger.Info("Published image delta: %d layer(s), smallest %d bytes (%.1f%% of the full layer).", len(d.layers), smallest, pct)
	return nil
}

// imageForDelta resolves the single image manifest referenced by an old tag,
// accepting both a bare manifest and an image index.
func imageForDelta(ref name.Reference, desc *v1.Descriptor, arch string, authOpt remote.Option) (v1.Image, error) {
	switch desc.MediaType {
	case types.OCIImageIndex, types.DockerManifestList:
		idx, err := remote.Index(ref, authOpt)
		if err != nil {
			return nil, fmt.Errorf("failed to load previous image index: %w", err)
		}
		im, err := idx.IndexManifest()
		if err != nil {
			return nil, fmt.Errorf("failed to read previous image index: %w", err)
		}
		chosen, err := chooseImageManifest(im.Manifests, arch)
		if err != nil {
			return nil, err
		}
		img, err := idx.Image(chosen.Digest)
		if err != nil {
			return nil, fmt.Errorf("failed to load previous image manifest: %w", err)
		}
		return img, nil
	default:
		img, err := remote.Image(ref, authOpt)
		if err != nil {
			return nil, fmt.Errorf("failed to load previous image: %w", err)
		}
		return img, nil
	}
}

// newChainDelta records the source chain for a target that has no usable
// delta. Flatpak finds no matching layer and falls back to the full layer, but
// the next release can still walk through the recorded sources.
func newChainDelta(sources []deltaSource, target v1.Hash) (*imageDelta, error) {
	sourceDigests := make([]v1.Hash, len(sources))
	for i := range sources {
		sourceDigests[i] = sources[i].desc.Digest
	}
	manifestBytes, manifestDigest, manifestSize, err := buildDeltaManifest(nil, sourceDigests, target)
	if err != nil {
		return nil, err
	}
	return &imageDelta{
		layers:         nil,
		manifestBytes:  manifestBytes,
		manifestDigest: manifestDigest,
		manifestSize:   manifestSize,
		targetDigest:   target,
	}, nil
}

// buildDeltaManifest serializes the manifest Flatpak resolves through
// _deltaindex.
func buildDeltaManifest(layers []v1.Descriptor, sources []v1.Hash, target v1.Hash) ([]byte, v1.Hash, int64, error) {
	sourceStrings := make([]string, len(sources))
	for i := range sources {
		sourceStrings[i] = sources[i].String()
	}
	sourcesJSON, err := json.Marshal(sourceStrings)
	if err != nil {
		return nil, v1.Hash{}, 0, fmt.Errorf("failed to marshal delta sources: %w", err)
	}
	configDigest, configSize, err := v1.SHA256(bytes.NewReader(deltaConfig))
	if err != nil {
		return nil, v1.Hash{}, 0, fmt.Errorf("failed to hash delta config: %w", err)
	}

	if layers == nil {
		layers = []v1.Descriptor{}
	}
	manifest := v1.Manifest{
		SchemaVersion: 2,
		MediaType:     types.OCIManifestSchema1,
		Config: v1.Descriptor{
			MediaType: deltaConfigMediaType,
			Size:      configSize,
			Digest:    configDigest,
		},
		Layers: layers,
		Annotations: map[string]string{
			deltaTargetAnnotation:  target.String(),
			deltaSourcesAnnotation: string(sourcesJSON),
		},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return nil, v1.Hash{}, 0, fmt.Errorf("failed to marshal delta manifest: %w", err)
	}
	digest, size, err := v1.SHA256(bytes.NewReader(body))
	if err != nil {
		return nil, v1.Hash{}, 0, fmt.Errorf("failed to hash delta manifest: %w", err)
	}
	return body, digest, size, nil
}

// rebuildDeltaIndex reconstructs _deltaindex from the _delta-* tag set.
// Rebuilding from tags instead of merging the previous index makes concurrent
// writers converge: a lost update is repaired by the next writer. Steady-state
// rebuilds reuse cached descriptors for tags that are not the required one, so
// an externally moved non-required tag is picked up on a later full rebuild.
func rebuildDeltaIndex(repoRef name.Repository, authOpt remote.Option, requiredTag string, preserveForeign, refresh bool) error {
	indexRef := repoRef.Tag(deltaIndexTag)

	for attempt := 1; attempt <= deltaIndexRetries; attempt++ {
		tags, err := listDeltaTags(repoRef, authOpt)
		if err != nil {
			if attempt == deltaIndexRetries {
				return fmt.Errorf("failed to list delta tags: %w", err)
			}
			sleepDeltaRetry(attempt)
			continue
		}
		// The tag listing may lag the manifest PUT on some registries. Never
		// publish an index that omits the delta this push just created.
		if requiredTag != "" && !containsString(tags, requiredTag) {
			sleepDeltaRetry(attempt)
			continue
		}

		existing, err := loadDeltaIndex(indexRef, authOpt)
		if err != nil {
			if attempt == deltaIndexRetries {
				return err
			}
			sleepDeltaRetry(attempt)
			continue
		}

		descriptors, err := deltaDescriptors(repoRef, tags, existing, requiredTag, refresh, authOpt)
		if err != nil {
			if attempt == deltaIndexRetries {
				return err
			}
			sleepDeltaRetry(attempt)
			continue
		}

		tagTargets := make(map[string]bool, len(tags))
		for _, tag := range tags {
			tagTargets["sha256:"+strings.TrimPrefix(tag, deltaTagPrefix)] = true
		}
		merged := mergeDeltaDescriptors(descriptors, existing, tagTargets, preserveForeign)
		if current, err := loadDeltaIndex(indexRef, authOpt); err == nil && current != nil && descriptorsEqual(current.Manifests, merged) {
			return nil
		}

		body, digest, size, err := buildDeltaIndex(merged)
		if err != nil {
			return err
		}
		indexDesc := &remote.Descriptor{
			Descriptor: v1.Descriptor{
				MediaType: types.OCIImageIndex,
				Size:      size,
				Digest:    digest,
			},
			Manifest: body,
		}
		if err := remote.Put(indexRef, indexDesc, authOpt); err != nil {
			if attempt == deltaIndexRetries {
				return fmt.Errorf("failed to publish delta index: %w", err)
			}
			sleepDeltaRetry(attempt)
			continue
		}

		// A tag may have been added while this writer was building the index.
		// Only finish when the tag set is unchanged and the published index
		// matches what that tag set describes.
		afterTags, err := listDeltaTags(repoRef, authOpt)
		if err == nil && stringSlicesEqual(tags, afterTags) {
			published, err := loadDeltaIndex(indexRef, authOpt)
			if err == nil && published != nil && descriptorsEqual(published.Manifests, merged) {
				return nil
			}
		}
		sleepDeltaRetry(attempt)
	}

	return fmt.Errorf("failed to rebuild delta index after %d attempts", deltaIndexRetries)
}

var errRequiredDeltaTag = errors.New("required delta tag is not readable")

func listDeltaTags(repoRef name.Repository, authOpt remote.Option) ([]string, error) {
	tags, err := remote.List(repoRef, authOpt)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if !strings.HasPrefix(tag, deltaTagPrefix) || seen[tag] {
			continue
		}
		if _, err := v1.NewHash("sha256:" + strings.TrimPrefix(tag, deltaTagPrefix)); err != nil {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	sort.Strings(out)
	return out, nil
}

// deltaDescriptors turns the tag set into index descriptors. Existing index
// entries are reused for tags that did not change, so a steady-state push only
// HEADs the tag it just created; a lost index falls back to HEADing every tag
// once. The required tag is always re-read so this push cannot silently omit
// its own delta. Tag names are trusted to describe their manifest's target;
// only aetherpak and explicitly foreign tags are expected here.
func deltaDescriptors(repoRef name.Repository, tags []string, existing *v1.IndexManifest, requiredTag string, refresh bool, authOpt remote.Option) ([]v1.Descriptor, error) {
	existingByTarget := make(map[string]v1.Descriptor, len(tags))
	if existing != nil {
		for _, d := range existing.Manifests {
			if target := d.Annotations[deltaTargetAnnotation]; target != "" {
				existingByTarget[target] = d
			}
		}
	}

	requiredTarget := "sha256:" + strings.TrimPrefix(requiredTag, deltaTagPrefix)
	descriptors := make([]v1.Descriptor, 0, len(tags))
	for _, tag := range tags {
		target, err := v1.NewHash("sha256:" + strings.TrimPrefix(tag, deltaTagPrefix))
		if err != nil {
			continue
		}
		isRequired := target.String() == requiredTarget
		if !refresh && !isRequired {
			if cached, ok := existingByTarget[target.String()]; ok {
				descriptors = append(descriptors, cached)
				continue
			}
		}

		desc, err := remote.Head(repoRef.Tag(tag), authOpt)
		if err != nil {
			if isNotFound(err) {
				if isRequired {
					return nil, fmt.Errorf("%w: %s", errRequiredDeltaTag, tag)
				}
				continue
			}
			// A cached entry is safer than dropping a delta because of a
			// transient registry error; the required tag normally has no cache
			// hit, so a genuinely new delta still fails and retries.
			if cached, ok := existingByTarget[target.String()]; ok {
				logger.Warn("WARNING: could not refresh delta manifest %s; keeping the existing index entry: %v", tag, err)
				descriptors = append(descriptors, cached)
				continue
			}
			return nil, fmt.Errorf("failed to read delta manifest %s: %w", tag, err)
		}
		descriptors = append(descriptors, v1.Descriptor{
			MediaType: types.OCIManifestSchema1,
			Size:      desc.Size,
			Digest:    desc.Digest,
			Annotations: map[string]string{
				deltaTargetAnnotation: target.String(),
			},
		})
	}
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].Annotations[deltaTargetAnnotation] < descriptors[j].Annotations[deltaTargetAnnotation]
	})
	return descriptors, nil
}

// mergeDeltaDescriptors keeps entries that are not managed by this tag naming
// scheme (for example deltas published by another tool), while replacing
// entries for targets that do have a _delta-* tag. Foreign entries are
// best-effort: a third-party writer can still race this read-modify-write, but
// aetherpak owns the index in the repositories it targets.
func mergeDeltaDescriptors(ours []v1.Descriptor, existing *v1.IndexManifest, tagTargets map[string]bool, preserveForeign bool) []v1.Descriptor {
	merged := append([]v1.Descriptor(nil), ours...)
	if preserveForeign && existing != nil {
		for _, d := range existing.Manifests {
			target := d.Annotations[deltaTargetAnnotation]
			if target == "" || !tagTargets[target] {
				merged = append(merged, d)
			}
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Annotations[deltaTargetAnnotation] < merged[j].Annotations[deltaTargetAnnotation]
	})
	return merged
}

func buildDeltaIndex(descriptors []v1.Descriptor) ([]byte, v1.Hash, int64, error) {
	if descriptors == nil {
		descriptors = []v1.Descriptor{}
	}
	manifest := v1.IndexManifest{
		SchemaVersion: 2,
		MediaType:     types.OCIImageIndex,
		Manifests:     descriptors,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return nil, v1.Hash{}, 0, fmt.Errorf("failed to marshal delta index: %w", err)
	}
	digest, size, err := v1.SHA256(bytes.NewReader(body))
	if err != nil {
		return nil, v1.Hash{}, 0, fmt.Errorf("failed to hash delta index: %w", err)
	}
	return body, digest, size, nil
}

func loadDeltaIndex(ref name.Reference, authOpt remote.Option) (*v1.IndexManifest, error) {
	idx, err := remote.Index(ref, authOpt)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read delta index: %w", err)
	}
	im, err := idx.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("failed to parse delta index: %w", err)
	}
	return im, nil
}

func descriptorsEqual(a, b []v1.Descriptor) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].MediaType != b[i].MediaType ||
			a[i].Size != b[i].Size ||
			a[i].Digest != b[i].Digest ||
			a[i].Annotations[deltaTargetAnnotation] != b[i].Annotations[deltaTargetAnnotation] {
			return false
		}
	}
	return true
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sleepDeltaRetry(attempt int) {
	base := time.Duration(attempt) * 100 * time.Millisecond
	jitter := time.Duration(rand.Int63n(int64(100 * time.Millisecond)))
	time.Sleep(base + jitter)
}

func putManifest(ref name.Reference, body []byte, digest v1.Hash, size int64, authOpt remote.Option) error {
	return remote.Put(ref, &remote.Descriptor{
		Descriptor: v1.Descriptor{
			MediaType: types.OCIManifestSchema1,
			Size:      size,
			Digest:    digest,
		},
		Manifest: body,
	}, authOpt)
}

func deltaManifestTag(target v1.Hash) string {
	return deltaTagPrefix + target.Hex
}

// deltaWorkRoot returns a disk-backed scratch directory next to the records
// directory, deliberately outside the records tree that CI archives. It never
// falls back to the system temp directory, which is often tmpfs.
func deltaWorkRoot(recordsDir string) (string, error) {
	base := recordsDir
	if base == "" {
		base = "."
	}
	root := filepath.Join(filepath.Dir(filepath.Clean(base)), ".aetherpak-work")
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", fmt.Errorf("failed to create delta work directory %s: %w", root, err)
	}
	return root, nil
}

func spoolLayer(layer v1.Layer, dir, name string) (string, error) {
	rc, err := layer.Compressed()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	f, err := os.CreateTemp(dir, name+"-*")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, rc); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func hashFile(path string) (v1.Hash, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return v1.Hash{}, 0, err
	}
	defer f.Close()
	return v1.SHA256(f)
}

func isNotFound(err error) bool {
	var terr *transport.Error
	return errors.As(err, &terr) && terr.StatusCode == http.StatusNotFound
}

// fileLayer exposes a file-backed blob as a v1.Layer. The delta is not a tar
// stream, so Compressed and Uncompressed return the same bytes. DiffID is the
// same digest as a consequence; upload paths do not use it.
type fileLayer struct {
	path      string
	digest    v1.Hash
	size      int64
	mediaType types.MediaType
}

func (l *fileLayer) Digest() (v1.Hash, error)             { return l.digest, nil }
func (l *fileLayer) DiffID() (v1.Hash, error)             { return l.digest, nil }
func (l *fileLayer) Size() (int64, error)                 { return l.size, nil }
func (l *fileLayer) MediaType() (types.MediaType, error)  { return l.mediaType, nil }
func (l *fileLayer) Compressed() (io.ReadCloser, error)   { return os.Open(l.path) }
func (l *fileLayer) Uncompressed() (io.ReadCloser, error) { return os.Open(l.path) }
