//go:build integration

package tests

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createMockFlatpakAppCustom(t *testing.T, repoPath string, appID string, arch string, branch string) {
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
	if err := os.WriteFile(helloScript, []byte("echo 'Hello'\n"), 0755); err != nil {
		t.Fatal(err)
	}

	metadata := fmt.Sprintf(`[Application]
name=%s
runtime=org.freedesktop.Platform/%s/23.08
sdk=org.freedesktop.Sdk/%s/23.08
command=hello
`, appID, arch, arch)

	if err := os.WriteFile(filepath.Join(tmpApp, "metadata"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Initialize temporary repo if not existing
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

	// Initialize destination repo if not existing
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		initCmd := exec.Command("ostree", "init", "--mode=archive", "--repo="+repoPath)
		if err := initCmd.Run(); err != nil {
			t.Fatalf("failed to init repo: %v", err)
		}
	}

	// Commit to destination repo with ref-binding
	rebindCmd := exec.Command("flatpak", "build-commit-from",
		"--src-repo="+tmpRepo,
		"--src-ref=temp-branch",
		"--no-update-summary",
		repoPath,
		fmt.Sprintf("app/%s/%s/%s", appID, arch, branch),
	)
	var rebindStderr bytes.Buffer
	rebindCmd.Stderr = &rebindStderr
	if err := rebindCmd.Run(); err != nil {
		t.Fatalf("flatpak build-commit-from failed: %s", rebindStderr.String())
	}
}

func TestPushOCIRefAutoDetection(t *testing.T) {
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

	// Spin up registry container via container runtime
	t.Log("Starting local OCI registry container...")
	containerName := "aetherpak-test-registry-autodetect"

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

	// Build CLI binary
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

	t.Run("Scenario 1: No config file exists and no app-id is passed (auto-detects refs from repo)", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "aetherpak-sc1-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		repoPath := filepath.Join(tempDir, "repo")
		recordsDir := filepath.Join(tempDir, "records")

		// Create mock apps
		createMockFlatpakAppCustom(t, repoPath, "org.example.App1", "x86_64", "stable")
		createMockFlatpakAppCustom(t, repoPath, "org.example.App2", "x86_64", "beta")

		pushCmd := exec.Command(binaryPath, "push-oci",
			"--registry=localhost:"+registryPort,
			"--oci-repository=aetherpak/test-app",
			"--repo-path="+repoPath,
			"--records-dir="+recordsDir,
			"--allow-unsigned",
		)
		pushCmd.Dir = tempDir
		var stdout, stderr bytes.Buffer
		pushCmd.Stdout = &stdout
		pushCmd.Stderr = &stderr

		if err := pushCmd.Run(); err != nil {
			t.Fatalf("push-oci failed: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
		}

		// Verify that records exist for both resolved targets
		rec1 := filepath.Join(recordsDir, "org.example.App1-x86_64", "record.json")
		rec2 := filepath.Join(recordsDir, "org.example.App2-x86_64", "record.json")

		if _, err := os.Stat(rec1); os.IsNotExist(err) {
			t.Errorf("Expected record for App1 to exist: %s", rec1)
		}
		if _, err := os.Stat(rec2); os.IsNotExist(err) {
			t.Errorf("Expected record for App2 to exist: %s", rec2)
		}
	})

	t.Run("Scenario 1b: No config file exists, no app-id is passed, and repo has no refs", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "aetherpak-sc1b-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		repoPath := filepath.Join(tempDir, "repo")
		recordsDir := filepath.Join(tempDir, "records")

		// Create an empty repo (no refs)
		initCmd := exec.Command("ostree", "init", "--mode=archive", "--repo="+repoPath)
		if err := initCmd.Run(); err != nil {
			t.Fatal(err)
		}

		pushCmd := exec.Command(binaryPath, "push-oci",
			"--registry=localhost:"+registryPort,
			"--oci-repository=aetherpak/test-app",
			"--repo-path="+repoPath,
			"--records-dir="+recordsDir,
			"--allow-unsigned",
		)
		pushCmd.Dir = tempDir
		var stdout, stderr bytes.Buffer
		pushCmd.Stdout = &stdout
		pushCmd.Stderr = &stderr

		err = pushCmd.Run()
		if err == nil {
			t.Fatal("Expected push-oci to fail because there are no refs and no config file")
		}

		expectedErrorMsg := "no application ID provided and no configuration file found"
		if !strings.Contains(stderr.String(), expectedErrorMsg) && !strings.Contains(stdout.String(), expectedErrorMsg) {
			t.Errorf("Expected error to contain %q, got: Stderr: %s, Stdout: %s", expectedErrorMsg, stderr.String(), stdout.String())
		}
	})

	t.Run("Scenario 2a: Config file exists, no app-id passed, repo has refs (should auto-detect and NOT use config)", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "aetherpak-sc2a-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		repoPath := filepath.Join(tempDir, "repo")
		recordsDir := filepath.Join(tempDir, "records")

		// Write config file referencing org.example.ConfigApp
		configData := []byte(fmt.Sprintf(`
registry: localhost:%s
oci_repository: aetherpak/test-app
apps:
  - id: org.example.ConfigApp
    manifest: apps/org.example.ConfigApp.json
    branch: stable
`, registryPort))
		if err := os.WriteFile(filepath.Join(tempDir, "aetherpak.yaml"), configData, 0644); err != nil {
			t.Fatal(err)
		}

		// Create repo with refs for org.example.RepoApp (different from config)
		createMockFlatpakAppCustom(t, repoPath, "org.example.RepoApp", "x86_64", "stable")

		pushCmd := exec.Command(binaryPath, "push-oci",
			"--repo-path="+repoPath,
			"--records-dir="+recordsDir,
			"--allow-unsigned",
		)
		pushCmd.Dir = tempDir
		var stdout, stderr bytes.Buffer
		pushCmd.Stdout = &stdout
		pushCmd.Stderr = &stderr

		if err := pushCmd.Run(); err != nil {
			t.Fatalf("push-oci failed: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
		}

		// Verify only RepoApp was pushed
		recRepoApp := filepath.Join(recordsDir, "org.example.RepoApp-x86_64", "record.json")
		recConfigApp := filepath.Join(recordsDir, "org.example.ConfigApp-x86_64", "record.json")

		if _, err := os.Stat(recRepoApp); os.IsNotExist(err) {
			t.Errorf("Expected RepoApp to be pushed and record to exist: %s", recRepoApp)
		}
		if _, err := os.Stat(recConfigApp); err == nil {
			t.Errorf("Expected ConfigApp NOT to be pushed, but record exists: %s", recConfigApp)
		}
	})

	t.Run("Scenario 2b: Config file exists, no app-id passed, repo has NO refs (should fallback to config)", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "aetherpak-sc2b-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		repoPath := filepath.Join(tempDir, "repo")
		recordsDir := filepath.Join(tempDir, "records")

		// Write config file referencing org.example.ConfigApp
		configData := []byte(fmt.Sprintf(`
registry: localhost:%s
oci_repository: aetherpak/test-app
apps:
  - id: org.example.ConfigApp
    manifest: apps/org.example.ConfigApp.json
    branch: beta
`, registryPort))
		if err := os.WriteFile(filepath.Join(tempDir, "aetherpak.yaml"), configData, 0644); err != nil {
			t.Fatal(err)
		}

		// Create empty repo
		initCmd := exec.Command("ostree", "init", "--mode=archive", "--repo="+repoPath)
		if err := initCmd.Run(); err != nil {
			t.Fatal(err)
		}

		pushCmd := exec.Command(binaryPath, "push-oci",
			"--repo-path="+repoPath,
			"--records-dir="+recordsDir,
			"--allow-unsigned",
		)
		pushCmd.Dir = tempDir
		var stdout, stderr bytes.Buffer
		pushCmd.Stdout = &stdout
		pushCmd.Stderr = &stderr

		err = pushCmd.Run()
		if err == nil {
			t.Fatal("Expected push-oci to fail because ref does not exist in repo, but it should have fallen back to config first")
		}

		// Check that it fell back to config and tried to build the config app branch
		expectedErrorMsg := "app/org.example.ConfigApp/x86_64/beta"
		if !strings.Contains(stderr.String(), expectedErrorMsg) && !strings.Contains(stdout.String(), expectedErrorMsg) {
			t.Errorf("Expected error to contain %q (proving fallback to config occurred), got: Stderr: %s, Stdout: %s", expectedErrorMsg, stderr.String(), stdout.String())
		}
	})

	t.Run("Scenario 3: Filter by --arch and --branch", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "aetherpak-sc3-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		repoPath := filepath.Join(tempDir, "repo")

		// Create mock apps
		// 1. App1 on x86_64 stable
		createMockFlatpakAppCustom(t, repoPath, "org.example.App1", "x86_64", "stable")
		// 2. App1 on aarch64 stable
		createMockFlatpakAppCustom(t, repoPath, "org.example.App1", "aarch64", "stable")
		// 3. App1 on x86_64 beta
		createMockFlatpakAppCustom(t, repoPath, "org.example.App1", "x86_64", "beta")

		// Test Scenario 3a: Filter by arch --arch aarch64
		{
			recordsDir := filepath.Join(tempDir, "records_arch")
			pushCmd := exec.Command(binaryPath, "push-oci",
				"--registry=localhost:"+registryPort,
				"--oci-repository=aetherpak/test-app",
				"--repo-path="+repoPath,
				"--records-dir="+recordsDir,
				"--arch=aarch64",
				"--allow-unsigned",
			)
			var stdout, stderr bytes.Buffer
			pushCmd.Stdout = &stdout
			pushCmd.Stderr = &stderr

			if err := pushCmd.Run(); err != nil {
				t.Fatalf("push-oci arch-filter failed: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
			}

			// Verify only aarch64 was pushed
			recAarch64 := filepath.Join(recordsDir, "org.example.App1-aarch64", "record.json")
			recX86_64 := filepath.Join(recordsDir, "org.example.App1-x86_64", "record.json")

			if _, err := os.Stat(recAarch64); os.IsNotExist(err) {
				t.Errorf("Expected aarch64 record to exist: %s", recAarch64)
			}
			if _, err := os.Stat(recX86_64); err == nil {
				t.Errorf("Expected x86_64 record NOT to exist: %s", recX86_64)
			}
		}

		// Test Scenario 3b: Filter by branch --branch beta
		{
			recordsDir := filepath.Join(tempDir, "records_branch")
			pushCmd := exec.Command(binaryPath, "push-oci",
				"--registry=localhost:"+registryPort,
				"--oci-repository=aetherpak/test-app",
				"--repo-path="+repoPath,
				"--records-dir="+recordsDir,
				"--branch=beta",
				"--allow-unsigned",
			)
			var stdout, stderr bytes.Buffer
			pushCmd.Stdout = &stdout
			pushCmd.Stderr = &stderr

			if err := pushCmd.Run(); err != nil {
				t.Fatalf("push-oci branch-filter failed: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
			}

			// Verify only beta was pushed (which is on x86_64)
			recBeta := filepath.Join(recordsDir, "org.example.App1-x86_64", "record.json")
			recAarch64 := filepath.Join(recordsDir, "org.example.App1-aarch64", "record.json")

			if _, err := os.Stat(recBeta); os.IsNotExist(err) {
				t.Errorf("Expected x86_64 (beta) record to exist: %s", recBeta)
			}
			if _, err := os.Stat(recAarch64); err == nil {
				t.Errorf("Expected aarch64 (stable) record NOT to exist: %s", recAarch64)
			}
		}

		// Test Scenario 3c: Filter by both --arch aarch64 --branch stable
		{
			recordsDir := filepath.Join(tempDir, "records_both")
			pushCmd := exec.Command(binaryPath, "push-oci",
				"--registry=localhost:"+registryPort,
				"--oci-repository=aetherpak/test-app",
				"--repo-path="+repoPath,
				"--records-dir="+recordsDir,
				"--arch=aarch64",
				"--branch=stable",
				"--allow-unsigned",
			)
			var stdout, stderr bytes.Buffer
			pushCmd.Stdout = &stdout
			pushCmd.Stderr = &stderr

			if err := pushCmd.Run(); err != nil {
				t.Fatalf("push-oci both-filter failed: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
			}

			// Verify only aarch64 stable was pushed
			recAarch64 := filepath.Join(recordsDir, "org.example.App1-aarch64", "record.json")
			recX86_64 := filepath.Join(recordsDir, "org.example.App1-x86_64", "record.json")

			if _, err := os.Stat(recAarch64); os.IsNotExist(err) {
				t.Errorf("Expected aarch64 record to exist: %s", recAarch64)
			}
			if _, err := os.Stat(recX86_64); err == nil {
				t.Errorf("Expected x86_64 record NOT to exist: %s", recX86_64)
			}
		}
	})

	t.Run("Scenario 4: Push a runtime (extension) ref and verify it succeeds using autodetect", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "aetherpak-sc4-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		repoPath := filepath.Join(tempDir, "repo")
		recordsDir := filepath.Join(tempDir, "records")

		// Create mock runtime
		createMockFlatpakRuntimeCustom(t, repoPath, "org.freedesktop.Sdk.Extension.mock", "x86_64", "stable")

		pushCmd := exec.Command(binaryPath, "push-oci",
			"--registry=localhost:"+registryPort,
			"--oci-repository=aetherpak/test-runtime",
			"--repo-path="+repoPath,
			"--records-dir="+recordsDir,
			"--allow-unsigned",
		)
		pushCmd.Dir = tempDir
		var stdout, stderr bytes.Buffer
		pushCmd.Stdout = &stdout
		pushCmd.Stderr = &stderr

		if err := pushCmd.Run(); err != nil {
			t.Fatalf("push-oci runtime push failed: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
		}

		// Verify record exists and has runtime ref
		rec := filepath.Join(recordsDir, "org.freedesktop.Sdk.Extension.mock-x86_64", "record.json")
		if _, err := os.Stat(rec); os.IsNotExist(err) {
			t.Fatalf("Expected record for runtime to exist: %s", rec)
		}

		data, err := os.ReadFile(rec)
		if err != nil {
			t.Fatalf("failed to read record: %v", err)
		}
		if !strings.Contains(string(data), "runtime/org.freedesktop.Sdk.Extension.mock/x86_64/stable") {
			t.Errorf("expected record to contain runtime ref, got: %s", string(data))
		}
	})
}

func createMockFlatpakRuntimeCustom(t *testing.T, repoPath string, appID string, arch string, branch string) {
	// Create temp directories
	tmpApp, err := os.MkdirTemp("", "aetherpak-mock-runtime-*")
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

	metadata := fmt.Sprintf(`[Runtime]
name=%s
`, appID)

	if err := os.WriteFile(filepath.Join(tmpApp, "metadata"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// Initialize temporary repo if not existing
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
		"-s", "Mock runtime commit",
		tmpApp,
	)
	var commitStderr bytes.Buffer
	commitCmd.Stderr = &commitStderr
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("ostree commit failed: %s", commitStderr.String())
	}

	// Initialize destination repo if not existing
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		initCmd := exec.Command("ostree", "init", "--mode=archive", "--repo="+repoPath)
		if err := initCmd.Run(); err != nil {
			t.Fatalf("failed to init repo: %v", err)
		}
	}

	// Commit to destination repo with ref-binding
	rebindCmd := exec.Command("flatpak", "build-commit-from",
		"--src-repo="+tmpRepo,
		"--src-ref=temp-branch",
		"--no-update-summary",
		repoPath,
		fmt.Sprintf("runtime/%s/%s/%s", appID, arch, branch),
	)
	var rebindStderr bytes.Buffer
	rebindCmd.Stderr = &rebindStderr
	if err := rebindCmd.Run(); err != nil {
		t.Fatalf("flatpak build-commit-from failed: %s", rebindStderr.String())
	}
}
