//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestE2EPushTwoBranchesSameAppArch publishes one app that has two branches on
// the same architecture in a single push-oci run, then builds the site and
// checks both branches reach the index. The dotted branch (2.54) also exercises
// the OCI tag sanitization end to end.
func TestE2EPushTwoBranchesSameAppArch(t *testing.T) {
	// 1. Verify environment and tools
	requiredTools := []string{"flatpak", "ostree"}
	for _, tool := range requiredTools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("Skipping integration test: missing required tool %q", tool)
		}
	}

	runtime, err := resolveContainerRuntime()
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}

	// Find a free TCP port for the registry
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find a free port: %v", err)
	}
	_, registryPort, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		l.Close()
		t.Fatalf("failed to parse registry port: %v", err)
	}
	l.Close()

	const appID = "org.example.AppBranches"
	const arch = "x86_64"
	branches := []string{"master", "2.54"}

	tempDir, err := os.MkdirTemp("", "aetherpak-multi-branch-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	repoPath := filepath.Join(tempDir, "repo")
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// 2. Spin up registry container via container runtime
	t.Log("Starting local OCI registry container...")
	containerName := "aetherpak-test-multi-branch-registry-" + registryPort

	_ = exec.Command(runtime, "stop", containerName).Run()
	_ = exec.Command(runtime, "rm", containerName).Run()

	runCmd := exec.Command(runtime, "run", "-d",
		"--name", containerName,
		"-p", registryPort+":5000",
		"-e", "REGISTRY_STORAGE_DELETE_ENABLED=true",
		"docker.io/library/registry:2",
	)
	var runStderr bytes.Buffer
	runCmd.Stderr = &runStderr
	if err := runCmd.Run(); err != nil {
		t.Fatalf("failed to spin up registry container (%v): %s", err, runStderr.String())
	}

	t.Cleanup(func() {
		t.Log("Tearing down local OCI registry...")
		_ = exec.Command(runtime, "stop", containerName).Run()
		_ = exec.Command(runtime, "rm", containerName).Run()
	})

	registryAddr := "127.0.0.1:" + registryPort
	t.Log("Waiting for registry to accept TCP connections...")
	if !waitForTCPPort(registryAddr, 30*time.Second) {
		t.Fatalf("registry failed to start on address: %s", registryAddr)
	}

	// 3. Compile the binary
	t.Log("Compiling aetherpak binary...")
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = ".."
	var buildStderr bytes.Buffer
	buildCmd.Stderr = &buildStderr
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to compile aetherpak binary (%v): %s", err, buildStderr.String())
	}
	binaryPath, err := filepath.Abs(filepath.Join("..", "bin", "aetherpak"))
	if err != nil {
		t.Fatal(err)
	}

	// 4. One app, two branches, same architecture.
	for _, branch := range branches {
		createMockFlatpakAppCustom(t, repoPath, appID, arch, branch)
	}

	// 5. A single push with no arch/branch filters must publish both branches.
	t.Log("Pushing both branches in one push-oci run...")
	pushCmd := exec.Command(binaryPath, "push-oci",
		"--registry=localhost:"+registryPort,
		"--oci-repository=aetherpak/test-multi-branch",
		"--repo-path="+repoPath,
		"--records-dir="+recordsDir,
		"--allow-unsigned",
	)
	pushCmd.Dir = tempDir
	var pushStdout, pushStderr bytes.Buffer
	pushCmd.Stdout = &pushStdout
	pushCmd.Stderr = &pushStderr
	if err := pushCmd.Run(); err != nil {
		t.Fatalf("push-oci failed: %v\nStdout: %s\nStderr: %s", err, pushStdout.String(), pushStderr.String())
	}

	// Both branches must land in distinct branch-qualified cells, and nothing
	// else (in particular no legacy <app>-<arch> cell).
	expectedTags := map[string]string{
		"master": "org_example_AppBranches-master-x86_64",
		"2.54":   "org_example_AppBranches-2_54-x86_64",
	}

	entries, err := os.ReadDir(recordsDir)
	if err != nil {
		t.Fatalf("failed to read records dir: %v", err)
	}
	var cellDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(recordsDir, entry.Name(), "record.json")); err == nil {
			cellDirs = append(cellDirs, entry.Name())
		}
	}
	if len(cellDirs) != len(branches) {
		t.Errorf("expected %d record cells, got %v", len(branches), cellDirs)
	}

	for _, branch := range branches {
		cell := filepath.Join(recordsDir, appID+"-"+branch+"-"+arch, "record.json")
		if _, err := os.Stat(cell); err != nil {
			t.Errorf("expected a record cell for branch %q: %v", branch, err)
			continue
		}

		// The dotted branch proves PR #162's tag sanitization reached the record.
		var rec struct {
			Tag string `json:"tag"`
		}
		data, err := os.ReadFile(cell)
		if err != nil {
			t.Fatalf("failed to read %s: %v", cell, err)
		}
		if err := json.Unmarshal(data, &rec); err != nil {
			t.Fatalf("failed to parse %s: %v", cell, err)
		}
		if rec.Tag != expectedTags[branch] {
			t.Errorf("record tag for branch %q = %q, want %q", branch, rec.Tag, expectedTags[branch])
		}
	}

	if _, err := os.Stat(filepath.Join(recordsDir, appID+"-"+arch)); !os.IsNotExist(err) {
		t.Errorf("expected no legacy %s-%s cell, stat err = %v", appID, arch, err)
	}

	// 6. Build the site. Point pages-url at a local 404 to keep the run offline
	// and independent of any live Pages deployment.
	pagesServer := httptest.NewServer(http.NotFoundHandler())
	defer pagesServer.Close()

	t.Log("Executing build-site...")
	siteCmd := exec.Command(binaryPath, "build-site",
		"--pages-url="+pagesServer.URL,
		"--records-dir="+recordsDir,
		"--site-dir="+siteDir,
		"--allow-unsigned",
	)
	siteCmd.Dir = tempDir
	var siteStdout, siteStderr bytes.Buffer
	siteCmd.Stdout = &siteStdout
	siteCmd.Stderr = &siteStderr
	if err := siteCmd.Run(); err != nil {
		t.Fatalf("build-site failed: %v\nStdout: %s\nStderr: %s", err, siteStdout.String(), siteStderr.String())
	}

	// 7. Both branches must reach the index and get a flatpakref file.
	staticPath := filepath.Join(siteDir, "index", "static")
	data, err := os.ReadFile(staticPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", staticPath, err)
	}
	for _, branch := range branches {
		ref := "app/" + appID + "/" + arch + "/" + branch
		if !strings.Contains(string(data), ref) {
			t.Errorf("expected index/static to contain ref %q, got: %s", ref, string(data))
		}

		refFile := filepath.Join(siteDir, "refs", appID+"-"+branch+".flatpakref")
		if _, err := os.Stat(refFile); err != nil {
			t.Errorf("expected a flatpakref for branch %q: %v", branch, err)
		}
	}
}
