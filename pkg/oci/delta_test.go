package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aetherpak/aetherpak/pkg/executil"
	tarpatch "github.com/containers/tar-diff/pkg/tar-patch"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// buildAppLayer returns an uncompressed tar layer with a single file. The
// content is what changes between two releases under test.
func buildAppLayer(content string, big bool) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	write := func(name string, body []byte) {
		_ = tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0644,
			Size:     int64(len(body)),
			ModTime:  time.Unix(0, 0),
			Typeflag: tar.TypeReg,
		})
		_, _ = tw.Write(body)
	}
	if big {
		// A large unchanged file keeps the synthetic layer realistic: the delta
		// should only carry the small changed file, not the whole layer.
		write("app/big.bin", bytes.Repeat([]byte("0123456789abcdef"), 16384))
	}
	write("app/content.txt", []byte(content))
	_ = tw.Close()
	return buf.Bytes()
}

func gzipLayer(data []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write(data)
	_ = gw.Close()
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

// setupMockPushExecutorWithLayer simulates `flatpak build-bundle --oci` with a
// real single-layer image so delta generation has something to diff.
func setupMockPushExecutorWithLayer(content string, big bool) executil.Executor {
	mockExec := executil.NewMockExecutor()
	mockExec.OnCommand = func(cmd *executil.MockCommand) {
		if cmd.Name != "flatpak" || len(cmd.Args) == 0 || cmd.Args[0] != "build-bundle" {
			return
		}
		cmd.RunFunc = func() error {
			var nonFlags []string
			for _, arg := range cmd.Args[1:] {
				if !strings.HasPrefix(arg, "-") {
					nonFlags = append(nonFlags, arg)
				}
			}
			if len(nonFlags) < 2 {
				return nil
			}
			ociDir := nonFlags[1]
			blobsDir := filepath.Join(ociDir, "blobs", "sha256")
			if err := os.MkdirAll(blobsDir, 0755); err != nil {
				return err
			}
			writeBlob := func(content []byte) (string, int, error) {
				hex := sha256Hex(content)
				if err := os.WriteFile(filepath.Join(blobsDir, hex), content, 0644); err != nil {
					return "", 0, err
				}
				return hex, len(content), nil
			}

			layerTar := buildAppLayer(content, big)
			diffID := sha256Hex(layerTar)
			layerGz := gzipLayer(layerTar)
			layerHex, layerSize, err := writeBlob(layerGz)
			if err != nil {
				return err
			}

			config := fmt.Sprintf(`{
				"architecture": "amd64",
				"os": "linux",
				"config": {"Labels": {"org.flatpak.ref": "app/org.example.App/x86_64/stable"}},
				"rootfs": {"type": "layers", "diff_ids": ["sha256:%s"]}
			}`, diffID)
			configHex, configSize, err := writeBlob([]byte(config))
			if err != nil {
				return err
			}

			manifest := fmt.Sprintf(`{
				"schemaVersion": 2,
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"config": {
					"mediaType": "application/vnd.oci.image.config.v1+json",
					"digest": "sha256:%s",
					"size": %d
				},
				"layers": [{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"digest": "sha256:%s",
					"size": %d
				}]
			}`, configHex, configSize, layerHex, layerSize)
			manifestHex, manifestSize, err := writeBlob([]byte(manifest))
			if err != nil {
				return err
			}

			index := fmt.Sprintf(`{
				"schemaVersion": 2,
				"manifests": [{
					"mediaType": "application/vnd.oci.image.manifest.v1+json",
					"digest": "sha256:%s",
					"size": %d
				}]
			}`, manifestHex, manifestSize)
			return os.WriteFile(filepath.Join(ociDir, "index.json"), []byte(index), 0644)
		}
	}
	return mockExec
}

func pushTestImage(t *testing.T, host, repo, recordsDir, content string, deltas, big bool) PushResult {
	return pushTestImageBack(t, host, repo, recordsDir, content, deltas, big, 0)
}

func pushTestImageBack(t *testing.T, host, repo, recordsDir, content string, deltas, big bool, deltaBack int) PushResult {
	t.Helper()
	opts := PushOptions{
		AppID:         "org.example.App",
		Arch:          "x86_64",
		Branch:        "stable",
		Registry:      host,
		OCIRepository: repo,
		RepoPath:      "repo",
		RecordsDir:    recordsDir,
		Insecure:      true,
		Executor:      setupMockPushExecutorWithLayer(content, big),
		AllowUnsigned: true,
		ImageDeltas:   deltas,
		DeltaBack:     deltaBack,
	}
	res, err := Push(opts)
	if err != nil {
		t.Fatalf("push %q failed: %v", content, err)
	}
	return res
}

func TestPushPublishesImageDelta(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()

	res1 := pushTestImage(t, host, "org.example.app", recordsDir, "version one", true, true)
	res2 := pushTestImage(t, host, "org.example.app", recordsDir, "version two", true, true)

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	index, err := remote.Index(repoRef.Tag(deltaIndexTag))
	if err != nil {
		t.Fatalf("expected %s tag: %v", deltaIndexTag, err)
	}
	im, err := index.IndexManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(im.Manifests) != 1 {
		t.Fatalf("expected one delta entry, got %d", len(im.Manifests))
	}
	if got := im.Manifests[0].Annotations[deltaTargetAnnotation]; got != res2.Digest {
		t.Fatalf("delta target = %q, want %q", got, res2.Digest)
	}

	deltaManifestDesc, err := remote.Get(repoRef.Digest(im.Manifests[0].Digest.String()))
	if err != nil {
		t.Fatal(err)
	}
	var deltaManifest v1.Manifest
	if err := json.Unmarshal(deltaManifestDesc.Manifest, &deltaManifest); err != nil {
		t.Fatal(err)
	}
	if got := deltaManifest.Annotations[deltaTargetAnnotation]; got != res2.Digest {
		t.Fatalf("delta manifest target = %q, want %q", got, res2.Digest)
	}
	if deltaManifest.MediaType != types.OCIManifestSchema1 {
		t.Fatalf("delta manifest media type = %q, want %q", deltaManifest.MediaType, types.OCIManifestSchema1)
	}
	if deltaManifest.Config.MediaType != deltaConfigMediaType {
		t.Fatalf("delta config media type = %q, want %q", deltaManifest.Config.MediaType, deltaConfigMediaType)
	}
	if len(deltaManifest.Layers) != 1 {
		t.Fatalf("expected one delta layer, got %d", len(deltaManifest.Layers))
	}
	if im.MediaType != types.OCIImageIndex {
		t.Fatalf("delta index media type = %q, want %q", im.MediaType, types.OCIImageIndex)
	}
	if im.Manifests[0].MediaType != types.OCIManifestSchema1 {
		t.Fatalf("delta index entry media type = %q, want %q", im.Manifests[0].MediaType, types.OCIManifestSchema1)
	}
	if _, err := remote.Head(repoRef.Tag(deltaManifestTag(mustHash(t, res2.Digest)))); err != nil {
		t.Fatalf("expected retained delta manifest tag: %v", err)
	}

	workRoot := filepath.Join(filepath.Dir(recordsDir), ".aetherpak-work")
	entries, err := os.ReadDir(workRoot)
	if err != nil {
		t.Fatalf("expected delta work directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("delta work directory was not cleaned: %v", entries)
	}

	oldImg, err := remote.Image(repoRef.Digest(res1.Digest))
	if err != nil {
		t.Fatal(err)
	}
	oldCfg, err := oldImg.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	newImg, err := remote.Image(repoRef.Digest(res2.Digest))
	if err != nil {
		t.Fatal(err)
	}
	newCfg, err := newImg.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}

	layer := deltaManifest.Layers[0]
	if got := layer.Annotations[deltaFromAnnotation]; got != oldCfg.RootFS.DiffIDs[0].String() {
		t.Fatalf("delta from = %q, want %q", got, oldCfg.RootFS.DiffIDs[0])
	}
	if got := layer.Annotations[deltaToAnnotation]; got != newCfg.RootFS.DiffIDs[0].String() {
		t.Fatalf("delta to = %q, want %q", got, newCfg.RootFS.DiffIDs[0])
	}
	if layer.MediaType != deltaLayerMediaType {
		t.Fatalf("delta layer media type = %q, want %q", layer.MediaType, deltaLayerMediaType)
	}

	deltaLayer, err := remote.Layer(repoRef.Digest(layer.Digest.String()))
	if err != nil {
		t.Fatal(err)
	}
	deltaReader, err := deltaLayer.Compressed()
	if err != nil {
		t.Fatal(err)
	}
	deltaBytes, err := io.ReadAll(deltaReader)
	if err != nil {
		t.Fatal(err)
	}
	_ = deltaReader.Close()

	// Flatpak only understands the v1 tar-diff stream; v2 zstd-dict ops would
	// make the delta unusable even though the round-trip below would pass.
	if len(deltaBytes) < 8 || string(deltaBytes[:8]) != "tardf1\n\x00" {
		t.Fatalf("delta has unexpected header %q; Flatpak requires tardf1", deltaBytes[:min(8, len(deltaBytes))])
	}

	oldDir := t.TempDir()
	extractTar(t, buildAppLayer("version one", true), oldDir)
	ds := tarpatch.NewFilesystemDataSource(oldDir)
	defer ds.Close()

	var reconstructed bytes.Buffer
	if err := tarpatch.Apply(bytes.NewReader(deltaBytes), ds, &reconstructed); err != nil {
		t.Fatalf("failed to apply delta: %v", err)
	}
	if !bytes.Equal(reconstructed.Bytes(), buildAppLayer("version two", true)) {
		t.Fatalf("reconstructed layer does not match the target layer")
	}
}

func TestPushWithoutImageDeltasPublishesNoDelta(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()

	pushTestImage(t, host, "org.example.app", recordsDir, "version one", false, true)
	pushTestImage(t, host, "org.example.app", recordsDir, "version two", false, true)

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Index(repoRef.Tag(deltaIndexTag)); err == nil {
		t.Fatalf("expected no %s tag when image deltas are disabled", deltaIndexTag)
	}
}

func TestPushImageDeltaMergesExistingEntries(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()

	pushTestImage(t, host, "org.example.app", recordsDir, "version one", true, true)
	res2 := pushTestImage(t, host, "org.example.app", recordsDir, "version two", true, true)
	res3 := pushTestImage(t, host, "org.example.app", recordsDir, "version three", true, true)

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	index, err := remote.Index(repoRef.Tag(deltaIndexTag))
	if err != nil {
		t.Fatal(err)
	}
	im, err := index.IndexManifest()
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]bool{}
	for _, m := range im.Manifests {
		targets[m.Annotations[deltaTargetAnnotation]] = true
	}
	if !targets[res2.Digest] || !targets[res3.Digest] {
		t.Fatalf("expected delta entries for v2 and v3, got %v", targets)
	}
	if len(im.Manifests) != 2 {
		t.Fatalf("expected two delta entries, got %d", len(im.Manifests))
	}
}

func TestPushPublishesDeltasFromEarlierReleases(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()

	res1 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version one", false, true, 1)
	res2 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version two", true, true, 1)
	res3 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version three", true, true, 2)
	res4 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version four", true, true, 2)

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	auth := remote.WithAuth(authn.Anonymous)

	v3Manifest := fetchDeltaManifest(t, repoRef, res3.Digest, auth)
	assertDeltaSources(t, v3Manifest, res2.Digest, res1.Digest)

	v4Manifest := fetchDeltaManifest(t, repoRef, res4.Digest, auth)
	assertDeltaSources(t, v4Manifest, res3.Digest, res2.Digest)
	if len(v4Manifest.Layers) != 2 {
		t.Fatalf("expected two v4 delta layers, got %d", len(v4Manifest.Layers))
	}

	diffIDs := map[string]string{
		res1.Digest: diffIDForDigest(t, repoRef, res1.Digest),
		res2.Digest: diffIDForDigest(t, repoRef, res2.Digest),
		res3.Digest: diffIDForDigest(t, repoRef, res3.Digest),
		res4.Digest: diffIDForDigest(t, repoRef, res4.Digest),
	}
	for _, layer := range v4Manifest.Layers {
		fromImage := layer.Annotations[deltaFromImageAnnotation]
		if fromImage != res2.Digest && fromImage != res3.Digest {
			t.Fatalf("v4 layer source = %q, want v3 or v2", fromImage)
		}
		if got := layer.Annotations[deltaFromAnnotation]; got != diffIDs[fromImage] {
			t.Fatalf("v4 layer from = %q, want %q for source %s", got, diffIDs[fromImage], fromImage)
		}
		if got := layer.Annotations[deltaToAnnotation]; got != diffIDs[res4.Digest] {
			t.Fatalf("v4 layer to = %q, want %q", got, diffIDs[res4.Digest])
		}
	}

	for _, source := range []string{res1.Digest, res2.Digest, res3.Digest} {
		if _, err := remote.Head(repoRef.Tag(deltaSourceTag(mustHash(t, source))), auth); err != nil {
			t.Fatalf("expected retained source tag for %s: %v", source, err)
		}
	}

	index, err := remote.Index(repoRef.Tag(deltaIndexTag), auth)
	if err != nil {
		t.Fatal(err)
	}
	im, err := index.IndexManifest()
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]int{}
	for _, m := range im.Manifests {
		targets[m.Annotations[deltaTargetAnnotation]]++
	}
	if targets[res4.Digest] != 1 {
		t.Fatalf("expected exactly one index entry for v4, got %d", targets[res4.Digest])
	}
}

func TestDeltaSourceImagesPrefersOrderedAnnotation(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	auth := remote.WithAuth(authn.Anonymous)

	target, _ := v1.NewHash("sha256:" + sha256Hex([]byte("target")))
	older, _ := v1.NewHash("sha256:" + sha256Hex([]byte("older")))
	immediate, _ := v1.NewHash("sha256:" + sha256Hex([]byte("immediate")))

	layers := []v1.Descriptor{
		{MediaType: deltaLayerMediaType, Size: 1, Digest: older, Annotations: map[string]string{deltaFromImageAnnotation: older.String()}},
		{MediaType: deltaLayerMediaType, Size: 1, Digest: immediate, Annotations: map[string]string{deltaFromImageAnnotation: immediate.String()}},
	}
	sourcesJSON, _ := json.Marshal([]string{immediate.String(), older.String()})
	manifest := v1.Manifest{
		SchemaVersion: 2,
		MediaType:     types.OCIManifestSchema1,
		Config:        v1.Descriptor{MediaType: deltaConfigMediaType, Size: 2, Digest: mustHash(t, "sha256:"+sha256Hex([]byte("{}")))},
		Layers:        layers,
		Annotations: map[string]string{
			deltaTargetAnnotation:  target.String(),
			deltaSourcesAnnotation: string(sourcesJSON),
		},
	}
	body, _ := json.Marshal(manifest)
	digest, size, _ := v1.SHA256(bytes.NewReader(body))
	if err := putManifest(repoRef.Tag(deltaManifestTag(target)), body, digest, size, auth); err != nil {
		t.Fatal(err)
	}

	got, err := deltaSourceImages(repoRef, target, auth)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != immediate || got[1] != older {
		t.Fatalf("ordered sources = %v, want [%s %s]", got, immediate, older)
	}

	// A manifest without the ordered annotation falls back to layer order.
	delete(manifest.Annotations, deltaSourcesAnnotation)
	body, _ = json.Marshal(manifest)
	digest, size, _ = v1.SHA256(bytes.NewReader(body))
	if err := putManifest(repoRef.Tag(deltaManifestTag(target)), body, digest, size, auth); err != nil {
		t.Fatal(err)
	}
	got, err = deltaSourceImages(repoRef, target, auth)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != older || got[1] != immediate {
		t.Fatalf("fallback sources = %v, want layer order [%s %s]", got, older, immediate)
	}
}

func TestDeltaSourceImagesFromChainOnlyManifest(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	auth := remote.WithAuth(authn.Anonymous)

	target, _ := v1.NewHash("sha256:" + sha256Hex([]byte("target")))
	older, _ := v1.NewHash("sha256:" + sha256Hex([]byte("older")))
	immediate, _ := v1.NewHash("sha256:" + sha256Hex([]byte("immediate")))
	body, digest, size, err := buildDeltaManifest(nil, []v1.Hash{immediate, older}, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := putManifest(repoRef.Tag(deltaManifestTag(target)), body, digest, size, auth); err != nil {
		t.Fatal(err)
	}
	got, err := deltaSourceImages(repoRef, target, auth)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != immediate || got[1] != older {
		t.Fatalf("chain-only sources = %v, want [%s %s]", got, immediate, older)
	}
}

func assertDeltaSources(t *testing.T, manifest v1.Manifest, want ...string) {
	t.Helper()
	raw := manifest.Annotations[deltaSourcesAnnotation]
	if raw == "" {
		t.Fatalf("delta manifest is missing the source list annotation")
	}
	var got []string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("failed to parse source list: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("source list = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("source list = %v, want %v", got, want)
		}
	}
}

func diffIDForDigest(t *testing.T, repoRef name.Repository, digest string) string {
	t.Helper()
	img, err := remote.Image(repoRef.Digest(digest))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RootFS.DiffIDs) != 1 {
		t.Fatalf("expected one diffID for %s, got %d", digest, len(cfg.RootFS.DiffIDs))
	}
	return cfg.RootFS.DiffIDs[0].String()
}

func fetchDeltaManifest(t *testing.T, repoRef name.Repository, target string, auth remote.Option) v1.Manifest {
	t.Helper()
	desc, err := remote.Get(repoRef.Tag(deltaManifestTag(mustHash(t, target))), auth)
	if err != nil {
		t.Fatal(err)
	}
	var manifest v1.Manifest
	if err := json.Unmarshal(desc.Manifest, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestPushImageDeltaSkipsIdenticalLayer(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()

	pushTestImage(t, host, "org.example.app", recordsDir, "same content", true, true)
	pushTestImage(t, host, "org.example.app", recordsDir, "same content", true, true)

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Index(repoRef.Tag(deltaIndexTag)); err == nil {
		t.Fatalf("expected no %s tag when the layer did not change", deltaIndexTag)
	}
}

func mustHash(t *testing.T, s string) v1.Hash {
	t.Helper()
	h, err := v1.NewHash(s)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestRebuildDeltaIndexConcurrent(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	auth := remote.WithAuth(authn.Anonymous)

	const workers = 12
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target, err := v1.NewHash("sha256:" + sha256Hex([]byte(fmt.Sprintf("target-%d", i))))
			if err != nil {
				errs <- err
				return
			}
			body := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","size":2,"digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"},"layers":[]}`)
			digest, size, err := v1.SHA256(bytes.NewReader(body))
			if err != nil {
				errs <- err
				return
			}
			if err := putManifest(repoRef.Tag(deltaManifestTag(target)), body, digest, size, auth); err != nil {
				errs <- err
				return
			}
			if err := rebuildDeltaIndex(repoRef, auth, deltaManifestTag(target), true, false); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent rebuild failed: %v", err)
	}

	index, err := remote.Index(repoRef.Tag(deltaIndexTag), auth)
	if err != nil {
		t.Fatalf("expected %s tag: %v", deltaIndexTag, err)
	}
	im, err := index.IndexManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(im.Manifests) != workers {
		t.Fatalf("expected %d delta entries after concurrent rebuilds, got %d", workers, len(im.Manifests))
	}
	seen := map[string]bool{}
	for _, m := range im.Manifests {
		seen[m.Annotations[deltaTargetAnnotation]] = true
	}
	for i := 0; i < workers; i++ {
		target, _ := v1.NewHash("sha256:" + sha256Hex([]byte(fmt.Sprintf("target-%d", i))))
		if !seen[target.String()] {
			t.Errorf("lost delta entry for %s", target)
		}
	}
}

func TestResolveDeltaSourceUsesChildManifest(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()

	res := pushTestImage(t, host, "org.example.app", recordsDir, "version one", false, true)

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	ref, desc, err := resolveDeltaSource(repoRef, "org_example_App-stable-x86_64", "amd64", remote.WithAuth(authn.Anonymous))
	if err != nil {
		t.Fatal(err)
	}
	if desc == nil {
		t.Fatal("expected a previous image descriptor")
	}
	if desc.MediaType != types.OCIManifestSchema1 {
		t.Fatalf("resolved media type = %q, want a concrete image manifest", desc.MediaType)
	}
	if desc.Digest.String() != res.Digest {
		t.Fatalf("resolved digest = %s, want child manifest %s", desc.Digest, res.Digest)
	}
	if ref.Identifier() != res.Digest {
		t.Fatalf("resolved ref = %s, want digest %s", ref.Identifier(), res.Digest)
	}
}

func minimalManifest() []byte {
	return []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","size":2,"digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"},"layers":[]}`)
}

func pushDeltaTag(t *testing.T, repoRef name.Repository, target v1.Hash, auth remote.Option) {
	t.Helper()
	body := minimalManifest()
	digest, size, err := v1.SHA256(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := putManifest(repoRef.Tag(deltaManifestTag(target)), body, digest, size, auth); err != nil {
		t.Fatal(err)
	}
}

func TestRebuildDeltaIndexPreservesForeignEntries(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	auth := remote.WithAuth(authn.Anonymous)

	foreignTarget, _ := v1.NewHash("sha256:" + sha256Hex([]byte("foreign-target")))
	foreignBody := minimalManifest()
	foreignDigest, foreignSize, err := v1.SHA256(bytes.NewReader(foreignBody))
	if err != nil {
		t.Fatal(err)
	}
	if err := putManifest(repoRef.Tag("foreign-delta"), foreignBody, foreignDigest, foreignSize, auth); err != nil {
		t.Fatal(err)
	}
	indexBody, indexDigest, indexSize, err := buildDeltaIndex([]v1.Descriptor{{
		MediaType:   types.OCIManifestSchema1,
		Size:        foreignSize,
		Digest:      foreignDigest,
		Annotations: map[string]string{deltaTargetAnnotation: foreignTarget.String()},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Put(repoRef.Tag(deltaIndexTag), &remote.Descriptor{
		Descriptor: v1.Descriptor{MediaType: types.OCIImageIndex, Size: indexSize, Digest: indexDigest},
		Manifest:   indexBody,
	}, auth); err != nil {
		t.Fatal(err)
	}

	ownTarget, _ := v1.NewHash("sha256:" + sha256Hex([]byte("own-target")))
	pushDeltaTag(t, repoRef, ownTarget, auth)
	if err := rebuildDeltaIndex(repoRef, auth, deltaManifestTag(ownTarget), true, false); err != nil {
		t.Fatalf("rebuild failed: %v", err)
	}

	index, err := remote.Index(repoRef.Tag(deltaIndexTag), auth)
	if err != nil {
		t.Fatal(err)
	}
	im, err := index.IndexManifest()
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]bool{}
	for _, m := range im.Manifests {
		targets[m.Annotations[deltaTargetAnnotation]] = true
	}
	if !targets[foreignTarget.String()] {
		t.Errorf("foreign delta entry was dropped")
	}
	if !targets[ownTarget.String()] {
		t.Errorf("own delta entry is missing")
	}
	if len(im.Manifests) != 2 {
		t.Fatalf("expected two entries, got %d", len(im.Manifests))
	}
}

func TestRebuildDeltaIndexFailsWhenRequiredTagIsUnreadable(t *testing.T) {
	inner := registry.New()
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/v2/org.example.app/manifests/"+deltaTagPrefix) {
			http.NotFound(w, r)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	auth := remote.WithAuth(authn.Anonymous)
	target, _ := v1.NewHash("sha256:" + sha256Hex([]byte("phantom-target")))
	pushDeltaTag(t, repoRef, target, auth)

	if err := rebuildDeltaIndex(repoRef, auth, deltaManifestTag(target), true, false); err == nil {
		t.Fatalf("expected rebuild to fail while the required manifest is unreadable")
	}
	if _, err := remote.Index(repoRef.Tag(deltaIndexTag), auth); err == nil {
		t.Fatalf("expected no %s tag to be published", deltaIndexTag)
	}
}

func TestDeltaWorkRootTrailingSlash(t *testing.T) {
	t.Chdir(t.TempDir())
	got, err := deltaWorkRoot("records/")
	if err != nil {
		t.Fatal(err)
	}
	if got != ".aetherpak-work" {
		t.Fatalf("deltaWorkRoot(%q) = %q, want .aetherpak-work", "records/", got)
	}
}

func extractTar(t *testing.T, data []byte, dir string) {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		path := filepath.Join(dir, hdr.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
}
