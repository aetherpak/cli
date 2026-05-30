package importer

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aetherpak/aetherpak/pkg/executil"
	"github.com/aetherpak/aetherpak/pkg/logger"
)

// maxBundleSize limits bundle download size to prevent resource exhaustion.
// It is a variable so it can be overridden in tests.
var maxBundleSize int64 = 10 * 1024 * 1024 * 1024 // 10 GB

// ImportOptions contains options for importing external flatpak bundles.
type ImportOptions struct {
	AppID        string
	Arch         string
	Branch       string
	BundleURL    string
	BundleSHA256 string
	BundlePath   string // local path override
	RepoPath     string // destination OSTree repo (default "repo")
	Executor     executil.Executor
}

// Import downloads (if necessary), verifies, and imports a bundle, performing channel rebinding.
func Import(opts ImportOptions) error {
	if opts.Executor == nil {
		opts.Executor = executil.NewOSExecutor()
	}
	logger.Info("Executing import for application: %s (arch: %s, branch: %s)", opts.AppID, opts.Arch, opts.Branch)

	targetPath := opts.BundlePath
	if targetPath == "" {
		if opts.BundleURL == "" {
			return fmt.Errorf("either bundle-url or bundle-path must be specified")
		}

		// Download the bundle
		tmpFile, err := os.CreateTemp(logger.TempDir(), fmt.Sprintf("aetherpak-%s-*.flatpak", opts.AppID))
		if err != nil {
			return fmt.Errorf("failed to create temporary file: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		logger.Info("Downloading bundle from: %s", opts.BundleURL)
		client := &http.Client{
			Timeout: 30 * time.Minute,
		}
		resp, err := client.Get(opts.BundleURL)
		if err != nil {
			return fmt.Errorf("failed to download bundle: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to download bundle, status: %s", resp.Status)
		}

		// Calculate SHA-256 during download
		hasher := sha256.New()
		writer := io.MultiWriter(tmpFile, hasher)

		limitReader := io.LimitReader(resp.Body, maxBundleSize)
		n, err := io.Copy(writer, limitReader)
		if err != nil {
			return fmt.Errorf("failed to write bundle: %w", err)
		}

		if n >= maxBundleSize {
			var oneByte [1]byte
			if _, readErr := resp.Body.Read(oneByte[:]); readErr != io.EOF {
				return fmt.Errorf("bundle download exceeded maximum size limit of %d bytes", maxBundleSize)
			}
		}

		checksum := fmt.Sprintf("%x", hasher.Sum(nil))
		logger.Debug("Calculated SHA-256: %s", checksum)

		if opts.BundleSHA256 != "" && checksum != opts.BundleSHA256 {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", opts.BundleSHA256, checksum)
		}
		logger.Info("SHA-256 checksum verified successfully.")
		targetPath = tmpFile.Name()
	} else {
		logger.Info("Using local bundle path: %s", targetPath)
		if opts.BundleSHA256 != "" {
			file, err := os.Open(targetPath)
			if err != nil {
				return fmt.Errorf("failed to open bundle: %w", err)
			}
			defer file.Close()

			hasher := sha256.New()
			if _, err := io.Copy(hasher, file); err != nil {
				return fmt.Errorf("failed to calculate checksum: %w", err)
			}
			checksum := fmt.Sprintf("%x", hasher.Sum(nil))
			if checksum != opts.BundleSHA256 {
				return fmt.Errorf("checksum mismatch: expected %s, got %s", opts.BundleSHA256, checksum)
			}
			logger.Info("SHA-256 checksum verified successfully.")
		}
	}

	// 1. Create a scratch directory
	scratchDir, err := os.MkdirTemp("", "aetherpak-scratch-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary scratch directory: %w", err)
	}
	defer os.RemoveAll(scratchDir)

	// 2. Initialize scratch archive OSTree repo
	logger.Debug("Initializing scratch OSTree repo at: %s", scratchDir)
	initCmd := opts.Executor.Command("ostree", "--repo="+scratchDir, "init", "--mode=archive-z2")
	var initStderr bytes.Buffer
	initCmd.SetStderr(&initStderr)
	if err := initCmd.Run(); err != nil {
		return fmt.Errorf("failed to initialize scratch ostree repo (%w): %s", err, initStderr.String())
	}

	// 3. Import bundle into scratch repo
	logger.Info("Importing Flatpak bundle %s into scratch repo...", targetPath)
	importCmd := opts.Executor.Command("flatpak", "build-import-bundle", scratchDir, targetPath)
	var importStderr bytes.Buffer
	importCmd.SetStderr(&importStderr)
	if err := importCmd.Run(); err != nil {
		return fmt.Errorf("failed to import bundle into scratch repo (%w): %s", err, importStderr.String())
	}

	// 4. Resolve imported application ref
	logger.Debug("Resolving ref from scratch repo...")
	refsCmd := opts.Executor.Command("ostree", "refs", "--repo="+scratchDir)
	var refsStdout, refsStderr bytes.Buffer
	refsCmd.SetStdout(&refsStdout)
	refsCmd.SetStderr(&refsStderr)
	if err := refsCmd.Run(); err != nil {
		return fmt.Errorf("failed to list refs in scratch repo (%w): %s", err, refsStderr.String())
	}

	lines := strings.Split(refsStdout.String(), "\n")
	var srcRef string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "app/") {
			srcRef = trimmed
			break
		}
	}

	if srcRef == "" {
		return fmt.Errorf("no application ref (app/*) found in imported bundle")
	}
	logger.Info("Resolved imported ref: %s", srcRef)

	// Coordinates default to the bundle's ref (app/<id>/<arch>/<branch>); explicit options override.
	srcParts := strings.Split(srcRef, "/")
	if len(srcParts) != 4 {
		return fmt.Errorf("unexpected source ref format: %q", srcRef)
	}
	appID := opts.AppID
	if appID == "" {
		appID = srcParts[1]
	}
	arch := opts.Arch
	if arch == "" {
		arch = srcParts[2]
	}
	branch := opts.Branch
	if branch == "" {
		branch = srcParts[3]
	}

	// 5. Rebind branch/channel commit into destination repository
	repoPath := opts.RepoPath
	if repoPath == "" {
		repoPath = "repo"
	}
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return fmt.Errorf("failed to create target repo directory: %w", err)
	}

	// If dest repo config is missing, initialize it
	if _, err := os.Stat(filepath.Join(repoPath, "config")); os.IsNotExist(err) {
		logger.Debug("Initializing target OSTree repo at: %s", repoPath)
		targetInitCmd := opts.Executor.Command("ostree", "--repo="+repoPath, "init", "--mode=archive-z2")
		if err := targetInitCmd.Run(); err != nil {
			return fmt.Errorf("failed to initialize target ostree repo: %w", err)
		}
	}

	destRef := fmt.Sprintf("app/%s/%s/%s", appID, arch, branch)
	logger.Info("Rebinding commit: %s -> %s", srcRef, destRef)

	rebindCmd := opts.Executor.Command("flatpak", "build-commit-from",
		"--src-repo="+scratchDir,
		"--src-ref="+srcRef,
		"--update-appstream",
		"--no-update-summary",
		repoPath,
		destRef,
	)
	var rebindStdout, rebindStderr bytes.Buffer
	rebindCmd.SetStdout(&rebindStdout)
	rebindCmd.SetStderr(&rebindStderr)
	if err := rebindCmd.Run(); err != nil {
		return fmt.Errorf("failed to rebind imported branch commit (%w): %s", err, rebindStderr.String())
	}

	logger.Info("Import and branch rebinding completed successfully.")
	return nil
}
