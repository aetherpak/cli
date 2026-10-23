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

	deltaTargetAnnotation = "io.github.containers.delta.target"
	deltaFromAnnotation   = "io.github.containers.delta.from"
	deltaToAnnotation     = "io.github.containers.delta.to"

	// deltaIndexRetries bounds the read-merge-write loop on the shared _deltaindex tag.
	deltaIndexRetries = 6
	// deltaMaxLayerSize keeps one release from staging multi-gigabyte layers in
	// the workspace. Flatpak application layers are normally far below this.
	deltaMaxLayerSize = int64(2 << 30)
)

// deltaConfig is the empty configuration object required by the delta image
// manifest specification.
var deltaConfig = []byte("{}")

// imageDelta holds the local artifacts needed to publish one delta after the
// target image has been pushed.
type imageDelta struct {
	dir            string
	blobPath       string
	blobDigest     v1.Hash
	blobSize       int64
	layerSize      int64
	manifestBytes  []byte
	manifestDigest v1.Hash
	manifestSize   int64
	targetDigest   v1.Hash
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

// prepareImageDelta generates a delta from oldRef to newImg. It returns nil
// when a delta would not be smaller than the full layer.
func prepareImageDelta(oldRef name.Reference, oldDesc *v1.Descriptor, arch string, newImg v1.Image, newDigest v1.Hash, authOpt remote.Option, workDir string) (*imageDelta, error) {
	newCfg, err := newImg.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to read target image config: %w", err)
	}
	// Flatpak records one xa.diff-id for the deployment and uses it as the
	// delta source for every layer. Its OCI images are single-layer, so refuse
	// to guess for multi-layer images.
	if len(newCfg.RootFS.DiffIDs) != 1 {
		logger.Info("Target image has %d layers; image deltas currently support single-layer images only. Skipping.", len(newCfg.RootFS.DiffIDs))
		return nil, nil
	}
	newDiffID := newCfg.RootFS.DiffIDs[0]

	oldImg, err := imageForDelta(oldRef, oldDesc, arch, authOpt)
	if err != nil {
		return nil, err
	}

	oldCfg, err := oldImg.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to read previous image config: %w", err)
	}
	if len(oldCfg.RootFS.DiffIDs) != 1 {
		logger.Info("Previous image has %d layers; image deltas currently support single-layer images only. Skipping.", len(oldCfg.RootFS.DiffIDs))
		return nil, nil
	}
	oldDiffID := oldCfg.RootFS.DiffIDs[0]

	if oldDiffID == newDiffID {
		logger.Info("Previous and target layers are identical; skipping image delta.")
		return nil, nil
	}

	oldLayers, err := oldImg.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to read previous image layers: %w", err)
	}
	newLayers, err := newImg.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to read target image layers: %w", err)
	}
	if len(oldLayers) != 1 || len(newLayers) != 1 {
		logger.Info("Image has %d old / %d new layers; image deltas currently support single-layer images only. Skipping.", len(oldLayers), len(newLayers))
		return nil, nil
	}

	newLayerSize, err := newLayers[0].Size()
	if err != nil {
		return nil, fmt.Errorf("failed to read target layer size: %w", err)
	}
	oldLayerSize, err := oldLayers[0].Size()
	if err != nil {
		return nil, fmt.Errorf("failed to read previous layer size: %w", err)
	}
	if newLayerSize > deltaMaxLayerSize || oldLayerSize > deltaMaxLayerSize {
		logger.Info("Skipping image delta: layer exceeds the %d MiB work limit.", deltaMaxLayerSize/(1<<20))
		return nil, nil
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

	oldLayerPath, err := spoolLayer(oldLayers[0], dir, "old")
	if err != nil {
		return nil, fmt.Errorf("failed to stage previous layer: %w", err)
	}
	newLayerPath, err := spoolLayer(newLayers[0], dir, "new")
	if err != nil {
		return nil, fmt.Errorf("failed to stage target layer: %w", err)
	}

	deltaPath := filepath.Join(dir, "delta.tardiff")
	oldFile, err := os.Open(oldLayerPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open staged previous layer: %w", err)
	}
	defer oldFile.Close()
	newFile, err := os.Open(newLayerPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open staged target layer: %w", err)
	}
	defer newFile.Close()
	deltaFile, err := os.Create(deltaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create delta file: %w", err)
	}

	opts := tardiff.NewOptions()
	// Flatpak's apply implementation understands the v1 (bsdiff) format. Keep
	// the generator on that path so deltas remain consumable by current clients.
	opts.SetBinaryDiffMethod(tardiff.BinaryDiffBsdiff)
	// Keep tar-diff's analysis scratch on the same disk-backed workspace.
	opts.SetTmpDir(dir)

	logger.Info("Generating image delta from %s to %s...", oldDiffID, newDiffID)
	if err := tardiff.Diff([]io.ReadSeeker{oldFile}, newFile, deltaFile, opts); err != nil {
		_ = deltaFile.Close()
		return nil, fmt.Errorf("failed to generate image delta: %w", err)
	}
	if err := deltaFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize delta file: %w", err)
	}

	deltaStat, err := os.Stat(deltaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat delta file: %w", err)
	}
	deltaSize := deltaStat.Size()
	if deltaSize >= newLayerSize {
		logger.Info("Delta is not smaller than the full layer (%d >= %d bytes); skipping image delta.", deltaSize, newLayerSize)
		return nil, nil
	}

	deltaDigest, _, err := hashFile(deltaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to hash delta: %w", err)
	}

	layerDesc := v1.Descriptor{
		MediaType: deltaLayerMediaType,
		Size:      deltaSize,
		Digest:    deltaDigest,
		Annotations: map[string]string{
			deltaFromAnnotation: oldDiffID.String(),
			deltaToAnnotation:   newDiffID.String(),
		},
	}
	manifestBytes, manifestDigest, manifestSize, err := buildDeltaManifest(layerDesc, newDigest)
	if err != nil {
		return nil, err
	}

	keep = true
	return &imageDelta{
		dir:            dir,
		blobPath:       deltaPath,
		blobDigest:     deltaDigest,
		blobSize:       deltaSize,
		layerSize:      newLayerSize,
		manifestBytes:  manifestBytes,
		manifestDigest: manifestDigest,
		manifestSize:   manifestSize,
		targetDigest:   newDigest,
	}, nil
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

	blob := &fileLayer{
		path:      d.blobPath,
		digest:    d.blobDigest,
		size:      d.blobSize,
		mediaType: deltaLayerMediaType,
	}
	if err := remote.WriteLayer(repoRef, blob, authOpt); err != nil {
		return fmt.Errorf("failed to push delta blob: %w", err)
	}

	// Tagging the delta manifest keeps registries with garbage collection from
	// treating it as an orphan, and makes the tag set the source of truth for
	// rebuilding _deltaindex.
	manifestRef := repoRef.Tag(deltaManifestTag(d.targetDigest))
	if err := putManifest(manifestRef, d.manifestBytes, d.manifestDigest, d.manifestSize, authOpt); err != nil {
		return fmt.Errorf("failed to push delta manifest: %w", err)
	}

	if err := rebuildDeltaIndex(repoRef, authOpt, deltaManifestTag(d.targetDigest)); err != nil {
		return err
	}

	pct := 0.0
	if d.layerSize > 0 {
		pct = 100 * float64(d.blobSize) / float64(d.layerSize)
	}
	logger.Info("Published image delta: %d bytes (%.1f%% of the full layer).", d.blobSize, pct)
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

// buildDeltaManifest serializes the manifest Flatpak resolves through
// _deltaindex.
func buildDeltaManifest(layer v1.Descriptor, target v1.Hash) ([]byte, v1.Hash, int64, error) {
	configDigest, configSize, err := v1.SHA256(bytes.NewReader(deltaConfig))
	if err != nil {
		return nil, v1.Hash{}, 0, fmt.Errorf("failed to hash delta config: %w", err)
	}

	manifest := v1.Manifest{
		SchemaVersion: 2,
		MediaType:     types.OCIManifestSchema1,
		Config: v1.Descriptor{
			MediaType: deltaConfigMediaType,
			Size:      configSize,
			Digest:    configDigest,
		},
		Layers: []v1.Descriptor{layer},
		Annotations: map[string]string{
			deltaTargetAnnotation: target.String(),
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
func rebuildDeltaIndex(repoRef name.Repository, authOpt remote.Option, requiredTag string) error {
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

		descriptors, err := deltaDescriptors(repoRef, tags, existing, requiredTag, authOpt)
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
		merged := mergeDeltaDescriptors(descriptors, existing, tagTargets)
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
func deltaDescriptors(repoRef name.Repository, tags []string, existing *v1.IndexManifest, requiredTag string, authOpt remote.Option) ([]v1.Descriptor, error) {
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
		if !isRequired {
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
func mergeDeltaDescriptors(ours []v1.Descriptor, existing *v1.IndexManifest, tagTargets map[string]bool) []v1.Descriptor {
	merged := append([]v1.Descriptor(nil), ours...)
	if existing != nil {
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
