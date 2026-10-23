package oci

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func TestBackfillDeltasPublishesExistingImages(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()

	res1 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version one", false, true, 1)
	res2 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version two", false, true, 1)

	res, err := BackfillDeltas(BackfillOptions{
		Registry:      host,
		OCIRepository: "org.example.app",
		Insecure:      true,
		ToDigest:      res2.Digest,
		FromDigests:   []string{res1.Digest},
		WorkDir:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("backfill failed: %v", err)
	}
	if !res.Published || res.Layers != 1 {
		t.Fatalf("backfill result = %+v, want one published layer", res)
	}

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	auth := remote.WithAuth(authn.Anonymous)

	manifest := fetchDeltaManifest(t, repoRef, res2.Digest, auth)
	if len(manifest.Layers) != 1 {
		t.Fatalf("expected one delta layer, got %d", len(manifest.Layers))
	}
	if got := manifest.Layers[0].Annotations[deltaFromImageAnnotation]; got != res1.Digest {
		t.Fatalf("from-image = %q, want %q", got, res1.Digest)
	}
	if got := manifest.Annotations[deltaTargetAnnotation]; got != res2.Digest {
		t.Fatalf("target = %q, want %q", got, res2.Digest)
	}

	index, err := remote.Index(repoRef.Tag(deltaIndexTag), auth)
	if err != nil {
		t.Fatal(err)
	}
	im, err := index.IndexManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(im.Manifests) != 1 || im.Manifests[0].Annotations[deltaTargetAnnotation] != res2.Digest {
		t.Fatalf("unexpected delta index: %+v", im.Manifests)
	}
}

func TestBackfillDeltasDryRunDoesNotPublish(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()

	res1 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version one", false, true, 1)
	res2 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version two", false, true, 1)

	res, err := BackfillDeltas(BackfillOptions{
		Registry:      host,
		OCIRepository: "org.example.app",
		Insecure:      true,
		ToDigest:      res2.Digest,
		FromDigests:   []string{res1.Digest},
		WorkDir:       t.TempDir(),
		DryRun:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Published || res.Layers != 1 {
		t.Fatalf("dry-run result = %+v, want one generated layer and no publish", res)
	}

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Head(repoRef.Tag(deltaManifestTag(mustHash(t, res2.Digest))), remote.WithAuth(authn.Anonymous)); err == nil {
		t.Fatalf("dry run published a delta manifest")
	}
	if _, err := remote.Head(repoRef.Tag(deltaSourceTag(mustHash(t, res1.Digest))), remote.WithAuth(authn.Anonymous)); err == nil {
		t.Fatalf("dry run retained a source tag")
	}
}
