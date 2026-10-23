package oci

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func TestPruneDeltasRemovesMissingTargets(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()

	res1 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version one", false, true, 1)

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	auth := remote.WithAuth(authn.Anonymous)

	// Seed a delta entry before the next release so the index includes it. The
	// prune pass must remove the entry, not just the tag.
	missingTarget, _ := v1.NewHash("sha256:" + sha256Hex([]byte("missing-target")))
	body, digest, size, err := buildDeltaManifest(nil, nil, missingTarget)
	if err != nil {
		t.Fatal(err)
	}
	if err := putManifest(repoRef.Tag(deltaManifestTag(missingTarget)), body, digest, size, auth); err != nil {
		t.Fatal(err)
	}

	res2 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version two", true, true, 1)

	res, err := PruneDeltas(PruneOptions{
		Registry:      host,
		OCIRepository: "org.example.app",
		Insecure:      true,
	})
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if res.DeltaTags != 1 || !res.IndexRebuilt {
		t.Fatalf("prune result = %+v, want one delta tag and an index rebuild", res)
	}

	if _, err := remote.Head(repoRef.Tag(deltaManifestTag(missingTarget)), auth); err == nil {
		t.Fatalf("expected the missing-target delta tag to be deleted")
	}
	if _, err := remote.Head(repoRef.Tag(deltaManifestTag(mustHash(t, res2.Digest))), auth); err != nil {
		t.Fatalf("expected the v2 delta tag to remain: %v", err)
	}
	// v1 is the source of the v2 delta, so its retention tag must remain.
	if _, err := remote.Head(repoRef.Tag(deltaSourceTag(mustHash(t, res1.Digest))), auth); err != nil {
		t.Fatalf("expected source tag for %s to remain: %v", res1.Digest, err)
	}

	index, err := remote.Index(repoRef.Tag(deltaIndexTag), auth)
	if err != nil {
		t.Fatal(err)
	}
	im, err := index.IndexManifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range im.Manifests {
		if entry.Annotations[deltaTargetAnnotation] == missingTarget.String() {
			t.Fatalf("pruned target is still present in %s", deltaIndexTag)
		}
	}
	if len(im.Manifests) != 1 {
		t.Fatalf("expected one remaining index entry, got %d", len(im.Manifests))
	}
}

func TestPruneDeltasRemovesUnreferencedSourceTags(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()

	res1 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version one", false, true, 1)
	res2 := pushTestImageBack(t, host, "org.example.app", recordsDir, "version two", true, true, 1)
	pushTestImageBack(t, host, "org.example.app", recordsDir, "version three", true, true, 1)

	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	auth := remote.WithAuth(authn.Anonymous)

	// Remove v2's target image. The prune pass then drops the v2 delta, which
	// leaves v1 unreferenced and lets its source tag go.
	if err := remote.Delete(repoRef.Digest(res2.Digest), auth); err != nil {
		t.Fatal(err)
	}

	res, err := PruneDeltas(PruneOptions{
		Registry:      host,
		OCIRepository: "org.example.app",
		Insecure:      true,
	})
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if res.DeltaTags != 1 || res.SourceTags < 1 {
		t.Fatalf("prune result = %+v, want one delta and at least one source tag", res)
	}
	if _, err := remote.Head(repoRef.Tag(deltaSourceTag(mustHash(t, res1.Digest))), auth); err == nil {
		t.Fatalf("expected the unreferenced v1 source tag to be deleted")
	}
}

func TestPruneDeltasDryRunKeepsTags(t *testing.T) {
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")
	recordsDir := t.TempDir()

	pushTestImageBack(t, host, "org.example.app", recordsDir, "version one", false, true, 1)
	repoRef, err := name.NewRepository(host+"/org.example.app", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	auth := remote.WithAuth(authn.Anonymous)

	missingTarget, _ := v1.NewHash("sha256:" + sha256Hex([]byte("dry-run-target")))
	body, digest, size, err := buildDeltaManifest(nil, nil, missingTarget)
	if err != nil {
		t.Fatal(err)
	}
	if err := putManifest(repoRef.Tag(deltaManifestTag(missingTarget)), body, digest, size, auth); err != nil {
		t.Fatal(err)
	}

	res, err := PruneDeltas(PruneOptions{
		Registry:      host,
		OCIRepository: "org.example.app",
		Insecure:      true,
		DryRun:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.DeltaTags != 1 || res.IndexRebuilt {
		t.Fatalf("dry-run result = %+v, want one candidate and no index rebuild", res)
	}
	if _, err := remote.Head(repoRef.Tag(deltaManifestTag(missingTarget)), auth); err != nil {
		t.Fatalf("dry run deleted the tag: %v", err)
	}
}
