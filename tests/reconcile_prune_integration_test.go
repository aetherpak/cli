//go:build integration

package tests

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestE2EReconcilePruning(t *testing.T) {
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

	// Create temp workspace
	tempDir, err := os.MkdirTemp("", "aetherpak-reconcile-e2e-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	repoPath := filepath.Join(tempDir, "repo")
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")

	// 2. Spin up registry container via container runtime
	t.Log("Starting local OCI registry container...")
	containerName := "aetherpak-test-reconcile-registry-" + registryPort

	// Pre-emptive cleanup
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

	// Wait for registry port to open
	t.Log("Waiting for registry to accept TCP connections...")
	registryAddr := "127.0.0.1:" + registryPort
	if !waitForTCPPort(registryAddr, 30*time.Second) {
		t.Fatalf("registry failed to start on address: %s", registryAddr)
	}

	// 3. Build mock flatpak apps
	t.Log("Building mock application repository commits...")
	createMockFlatpakApp(t, repoPath, "org.flatpak.AppActive", "1.0")

	// 4. Compile binary
	t.Log("Compiling aetherpak binary...")
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = ".."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to compile aetherpak binary")
	}
	binaryPath := filepath.Join("..", "bin", "aetherpak")

	// 5. Push AppActive to example/app-active
	t.Log("Pushing Active app...")
	pushCmd1 := exec.Command(binaryPath, "push-oci",
		"--registry=localhost:"+registryPort,
		"--oci-repository=example/app-active",
		"--repo-path="+repoPath,
		"--records-dir="+recordsDir,
		"--insecure",
		"--allow-unsigned",
	)
	if err := pushCmd1.Run(); err != nil {
		t.Fatalf("push-oci Active failed")
	}

	// Now build AppStale and push to example/app-stale
	createMockFlatpakApp(t, repoPath, "org.flatpak.AppStale", "1.0")
	t.Log("Pushing Stale app...")
	pushCmd2 := exec.Command(binaryPath, "push-oci",
		"--registry=localhost:"+registryPort,
		"--oci-repository=example/app-stale",
		"--repo-path="+repoPath,
		"--records-dir="+recordsDir,
		"--insecure",
		"--allow-unsigned",
	)
	if err := pushCmd2.Run(); err != nil {
		t.Fatalf("push-oci Stale failed")
	}

	// 5.5 Start a local HTTP server in Go to serve the index site
	t.Log("Starting web server...")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	webPort := strings.Split(listener.Addr().String(), ":")[1]

	server := &http.Server{
		Handler: http.FileServer(http.Dir(siteDir)),
	}
	defer server.Shutdown(context.Background())

	go func() {
		_ = server.Serve(listener)
	}()

	// 6. Run build-site initially to merge both records (active & stale)
	t.Log("Executing initial build-site...")
	siteCmd1 := exec.Command(binaryPath, "build-site",
		"--pages-url=http://127.0.0.1:"+webPort,
		"--records-dir="+recordsDir,
		"--site-dir="+siteDir,
		"--allow-unsigned",
	)
	if err := siteCmd1.Run(); err != nil {
		t.Fatalf("initial build-site failed")
	}

	// Verify both exist in index/static initially
	staticPath := filepath.Join(siteDir, "index", "static")
	data, err := os.ReadFile(staticPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "org.flatpak.AppActive") || !strings.Contains(string(data), "org.flatpak.AppStale") {
		t.Fatalf("both apps should be present initially, got: %s", string(data))
	}

	// 7. Write aetherpak.yaml config that only defines AppActive and sets active oci_repository
	t.Log("Writing custom aetherpak.yaml configuration...")
	configData := []byte(`
registry: localhost:` + registryPort + `
oci_repository: example/app-active
apps:
  - id: org.flatpak.AppActive
    manifest: dummy.json
    arches:
      - x86_64
`)
	if err := os.WriteFile("aetherpak.yaml", configData, 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove("aetherpak.yaml")

	// 8. Re-run build-site with --reconcile. It should load aetherpak.yaml config,
	// see that example/app-stale (org.flatpak.AppStale) is unconfigured/inactive, and prune it!
	t.Log("Executing build-site with --reconcile...")
	// Clear the records directory to simulate a new run where only AppActive is built (or reconcile-only run)
	if err := os.RemoveAll(recordsDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(recordsDir, 0755); err != nil {
		t.Fatal(err)
	}

	siteCmd2 := exec.Command(binaryPath, "build-site",
		"--pages-url=http://127.0.0.1:"+webPort,
		"--records-dir="+recordsDir,
		"--site-dir="+siteDir,
		"--allow-unsigned",
		"--reconcile",
	)
	var site2Stdout, site2Stderr bytes.Buffer
	siteCmd2.Stdout = &site2Stdout
	siteCmd2.Stderr = &site2Stderr
	if err := siteCmd2.Run(); err != nil {
		t.Fatalf("build-site with reconcile failed: \nStdout: %s\nStderr: %s", site2Stdout.String(), site2Stderr.String())
	}

	// 9. Verify that index/static now only contains AppActive, and AppStale was pruned!
	data2, err := os.ReadFile(staticPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data2)
	t.Logf("Reconciled Index:\n%s", content)

	if !strings.Contains(content, "org.flatpak.AppActive") {
		t.Error("expected index to retain org.flatpak.AppActive")
	}
	if strings.Contains(content, "org.flatpak.AppStale") {
		t.Error("expected index to prune org.flatpak.AppStale since it is inactive and unconfigured")
	}
}
