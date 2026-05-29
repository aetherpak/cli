//go:build integration

package tests

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

const (
	appID        = "org.flatpak.MockApp"
)

func TestEndToEndIntegration(t *testing.T) {
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
	requiredTools := []string{"flatpak", "ostree", "gpg"}
	for _, tool := range requiredTools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("Skipping integration test: missing required tool %q", tool)
		}
	}

	runtime, err := resolveContainerRuntime()
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}

	// Create workspaces
	tempDir, err := os.MkdirTemp("", "aetherpak-e2e-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	repoPath := filepath.Join(tempDir, "repo")
	recordsDir := filepath.Join(tempDir, "records")
	siteDir := filepath.Join(tempDir, "site")
	flatpakUserDir := filepath.Join(tempDir, "flatpak_user")

	if err := os.MkdirAll(flatpakUserDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Override FLATPAK_USER_DIR environment variable for isolated client sandbox
	t.Setenv("FLATPAK_USER_DIR", flatpakUserDir)

	// 2. Spin up registry container via container runtime
	t.Log("Starting local OCI registry container...")
	containerName := "aetherpak-test-registry-" + registryPort

	// Pre-emptive cleanup in case of a dirty previous run
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

	// 3. Generate dummy GPG signing key pair in-memory
	t.Log("Generating GPG keys...")
	entity, err := openpgp.NewEntity("AetherPak E2E", "Test key", "e2e@aetherpak.local", nil)
	if err != nil {
		t.Fatalf("failed to generate key entity: %v", err)
	}

	var privKeyBlock bytes.Buffer
	wPriv, err := armor.Encode(&privKeyBlock, openpgp.PrivateKeyType, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := entity.SerializePrivate(wPriv, nil); err != nil {
		t.Fatal(err)
	}
	wPriv.Close()

	var pubKeyBlock bytes.Buffer
	wPub, err := armor.Encode(&pubKeyBlock, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := entity.Serialize(wPub); err != nil {
		t.Fatal(err)
	}
	wPub.Close()

	// Write armored GPG key block
	gpgKeyPath := filepath.Join(tempDir, "key.asc")
	if err := os.WriteFile(gpgKeyPath, pubKeyBlock.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	gpgPrivKeyPath := filepath.Join(tempDir, "key.priv.asc")
	if err := os.WriteFile(gpgPrivKeyPath, privKeyBlock.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Build a mock Flatpak OSTree repository
	t.Log("Building mock application repository...")
	createMockFlatpakApp(t, repoPath, appID, "1.0")

	// 5. Build our own compiled CLI binary
	t.Log("Compiling aetherpak binary...")
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = ".."
	var buildStderr bytes.Buffer
	buildCmd.Stderr = &buildStderr
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to compile aetherpak binary (%v): %s", err, buildStderr.String())
	}
	binaryPath := filepath.Join("..", "bin", "aetherpak")

	// 6. Run push-oci using our binary
	t.Log("Executing push-oci...")
	pushCmd := exec.Command(binaryPath, "push-oci",
		"--app="+appID,
		"--registry=localhost:"+registryPort,
		"--oci-repository=aetherpak/mock-app",
		"--repo-path="+repoPath,
		"--records-dir="+recordsDir,
		"--gpg-key="+gpgPrivKeyPath,
		"--insecure",
	)
	var pushStdout, pushStderr bytes.Buffer
	pushCmd.Stdout = &pushStdout
	pushCmd.Stderr = &pushStderr
	if err := pushCmd.Run(); err != nil {
		t.Fatalf("push-oci command failed (%v): \nStdout: %s\nStderr: %s", err, pushStdout.String(), pushStderr.String())
	}

	// 7. Start a local HTTP server in Go to serve the index site
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

	// 8. Run build-site using our binary
	t.Log("Executing build-site...")
	siteCmd := exec.Command(binaryPath, "build-site",
		"--pages-url=http://127.0.0.1:"+webPort,
		"--records-dir="+recordsDir,
		"--site-dir="+siteDir,
		"--gpg-key="+gpgPrivKeyPath,
	)
	var siteStdout, siteStderr bytes.Buffer
	siteCmd.Stdout = &siteStdout
	siteCmd.Stderr = &siteStderr
	if err := siteCmd.Run(); err != nil {
		t.Fatalf("build-site command failed (%v): \nStdout: %s\nStderr: %s", err, siteStdout.String(), siteStderr.String())
	}

	// 9. Client remote-add and install verification
	t.Log("Verifying client installations...")
	remoteName := "mock-signed-remote"
	remoteAddCmd := exec.Command("flatpak", "remote-add",
		"--user",
		"--gpg-import="+gpgKeyPath,
		"--signature-lookaside=http://127.0.0.1:"+webPort+"/sigs",
		remoteName,
		"oci+http://127.0.0.1:"+webPort,
	)
	var addStderr bytes.Buffer
	remoteAddCmd.Stderr = &addStderr
	if err := remoteAddCmd.Run(); err != nil {
		t.Fatalf("failed to add flatpak remote (%v): %s", err, addStderr.String())
	}

	// List remote content to verify mapping
	lsCmd := exec.Command("flatpak", "remote-ls", "--user", remoteName)
	var lsStdout, lsStderr bytes.Buffer
	lsCmd.Stdout = &lsStdout
	lsCmd.Stderr = &lsStderr
	if err := lsCmd.Run(); err != nil {
		t.Fatalf("flatpak remote-ls failed (%v): %s", err, lsStderr.String())
	}
	if !strings.Contains(lsStdout.String(), appID) {
		t.Fatalf("expected app %s to be listed in remote-ls, got: %s", appID, lsStdout.String())
	}

	// Install application (no-deps to bypass pulling platform runtimes)
	t.Log("Installing Flatpak application...")
	installCmd := exec.Command("flatpak", "install", "--user", "-y", "--no-deps", remoteName, appID)
	var installStdout, installStderr bytes.Buffer
	installCmd.Stdout = &installStdout
	installCmd.Stderr = &installStderr
	if err := installCmd.Run(); err != nil {
		t.Fatalf("flatpak install failed (%v): \nStdout: %s\nStderr: %s", err, installStdout.String(), installStderr.String())
	}

	// Verify application info
	infoCmd := exec.Command("flatpak", "info", "--user", appID)
	var infoStdout bytes.Buffer
	infoCmd.Stdout = &infoStdout
	if err := infoCmd.Run(); err == nil && !strings.Contains(infoStdout.String(), appID) {
		t.Fatalf("expected app %s in flatpak info, got: %s", appID, infoStdout.String())
	}

	// 10. Verification of GPG signing validation gate (TAMPER test)
	t.Log("Uninstalling and cleaning user environment for GPG tamper validation check...")
	_ = exec.Command("flatpak", "uninstall", "--user", "-y", appID).Run()
	_ = exec.Command("flatpak", "remote-delete", "--user", remoteName).Run()
	_ = os.RemoveAll(flatpakUserDir)
	_ = os.MkdirAll(flatpakUserDir, 0755)

	// Tamper signature lookaside file
	t.Log("Tampering with the lookaside signature file...")
	matches, err := filepath.Glob(filepath.Join(siteDir, "sigs", "aetherpak", "mock-app@sha256=*"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("failed to find signatures in lookaside directory for tampering (err: %v, matches: %d)", err, len(matches))
	}
	digestPath := matches[0]
	sigFile := filepath.Join(digestPath, "signature-1")
	if err := os.WriteFile(sigFile, []byte("TAMPERED_SIGNATURE_DATA"), 0644); err != nil {
		t.Fatalf("failed to tamper signature: %v", err)
	}

	// Re-add remote
	remoteAddCmd = exec.Command("flatpak", "remote-add",
		"--user",
		"--gpg-import="+gpgKeyPath,
		"--signature-lookaside=http://127.0.0.1:"+webPort+"/sigs",
		remoteName,
		"oci+http://127.0.0.1:"+webPort,
	)
	_ = remoteAddCmd.Run()

	// Install MUST fail verification now
	t.Log("Verifying GPG validation blocks tampered installs...")
	tamperInstallCmd := exec.Command("flatpak", "install", "--user", "-y", "--no-deps", remoteName, appID)
	var tamperStderr bytes.Buffer
	tamperInstallCmd.Stderr = &tamperStderr
	err = tamperInstallCmd.Run()

	if err == nil {
		t.Fatal("flatpak install succeeded despite tampered signature lookup file!")
	}

	errStr := strings.ToLower(tamperStderr.String())
	if !strings.Contains(errStr, "signature") && !strings.Contains(errStr, "verify") && !strings.Contains(errStr, "valid") {
		t.Fatalf("flatpak install failed with unexpected error output: %s", tamperStderr.String())
	}
	t.Log("GPG validation check rejected tampered signature correctly.")
}

func resolveContainerRuntime() (string, error) {
	// Prefer podman
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman", nil
	}
	// Fallback to docker
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker", nil
	}
	return "", fmt.Errorf("no container runtime (podman or docker) found")
}

func waitForTCPPort(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func createMockFlatpakApp(t *testing.T, repoPath string, appID string, version string) {
	if err := os.RemoveAll(repoPath); err != nil {
		t.Fatal(err)
	}

	// Initialize Ostree repository
	initCmd := exec.Command("ostree", "init", "--mode=archive", "--repo="+repoPath)
	if err := initCmd.Run(); err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	// Create temp directories
	tmpApp, err := os.MkdirTemp("", "aetherpak-mock-app-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpApp)

	tmpRepo, err := os.MkdirTemp("", "aetherpak-mock-repo-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpRepo)

	filesDir := filepath.Join(tmpApp, "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}

	helloScript := filepath.Join(filesDir, "hello")
	if err := os.WriteFile(helloScript, []byte(fmt.Sprintf("echo 'Hello version %s'\n", version)), 0755); err != nil {
		t.Fatal(err)
	}

	metadata := fmt.Sprintf(`[Application]
name=%s
runtime=org.freedesktop.Platform/x86_64/23.08
sdk=org.freedesktop.Sdk/x86_64/23.08
command=hello
`, appID)

	if err := os.WriteFile(filepath.Join(tmpApp, "metadata"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Commit to temporary repo
	initTmpCmd := exec.Command("ostree", "init", "--mode=archive", "--repo="+tmpRepo)
	if err := initTmpCmd.Run(); err != nil {
		t.Fatal(err)
	}

	commitCmd := exec.Command("ostree", "commit",
		"--repo="+tmpRepo,
		"--branch=temp-branch",
		"--owner-uid=0",
		"--owner-gid=0",
		"--canonical-permissions",
		"--add-metadata-string=xa.metadata="+metadata,
		"-s", "Mock application commit",
		tmpApp,
	)
	var commitStderr bytes.Buffer
	commitCmd.Stderr = &commitStderr
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("ostree commit failed: %s", commitStderr.String())
	}

	// Commit to destination repo with ref-binding
	rebindCmd := exec.Command("flatpak", "build-commit-from",
		"--src-repo="+tmpRepo,
		"--src-ref=temp-branch",
		"--no-update-summary",
		repoPath,
		"app/"+appID+"/x86_64/stable",
	)
	var rebindStderr bytes.Buffer
	rebindCmd.Stderr = &rebindStderr
	if err := rebindCmd.Run(); err != nil {
		t.Fatalf("flatpak build-commit-from failed: %s", rebindStderr.String())
	}
}
