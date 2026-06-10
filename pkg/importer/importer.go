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
	"github.com/aetherpak/aetherpak/pkg/repoinfo"
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

		path, checksum, err := Fetch(opts.BundleURL, nil)
		if err != nil {
			return err
		}
		defer os.Remove(path)

		if opts.BundleSHA256 != "" && checksum != opts.BundleSHA256 {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", opts.BundleSHA256, checksum)
		}
		logger.Info("SHA-256 checksum verified successfully.")
		targetPath = path
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
		if strings.HasPrefix(trimmed, "app/") || strings.HasPrefix(trimmed, "runtime/") {
			srcRef = trimmed
			break
		}
	}

	if srcRef == "" {
		return fmt.Errorf("no application or runtime ref (app/* or runtime/*) found in imported bundle")
	}
	logger.Info("Resolved imported ref: %s", srcRef)

	// Coordinates default to the bundle's ref (<type>/<id>/<arch>/<branch>); explicit options override.
	srcParts := strings.Split(srcRef, "/")
	if len(srcParts) != 4 {
		return fmt.Errorf("unexpected source ref format: %q", srcRef)
	}
	refType := srcParts[0]
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

	destRef := fmt.Sprintf("%s/%s/%s/%s", refType, appID, arch, branch)
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

// ProgressFunc reports download progress. total is -1 when unknown.
type ProgressFunc func(downloaded, total int64)

// Fetch downloads url to a temp file under the logger temp dir, computing its
// SHA-256 during transfer. It returns the temp file path (caller must remove it)
// and the hex checksum. progress may be nil.
func Fetch(url string, progress ProgressFunc) (string, string, error) {
	tmpFile, err := os.CreateTemp(logger.TempDir(), "aetherpak-fetch-*.flatpak")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	var success bool
	defer func() {
		if !success {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
		} else {
			tmpFile.Close()
		}
	}()

	logger.Info("Downloading from: %s", url)
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to download, status: %s", resp.Status)
	}

	hasher := sha256.New()
	counter := &progressWriter{total: resp.ContentLength, fn: progress}
	writer := io.MultiWriter(tmpFile, hasher, counter)

	limitReader := io.LimitReader(resp.Body, maxBundleSize)
	n, err := io.Copy(writer, limitReader)
	if err != nil {
		return "", "", fmt.Errorf("failed to write download: %w", err)
	}
	if n >= maxBundleSize {
		var oneByte [1]byte
		if _, readErr := resp.Body.Read(oneByte[:]); readErr != io.EOF {
			return "", "", fmt.Errorf("download exceeded maximum size limit of %d bytes", maxBundleSize)
		}
	}

	checksum := fmt.Sprintf("%x", hasher.Sum(nil))
	logger.Debug("Calculated SHA-256: %s", checksum)
	success = true
	return tmpFile.Name(), checksum, nil
}

// progressWriter counts bytes written and forwards progress updates.
type progressWriter struct {
	written int64
	total   int64
	fn      ProgressFunc
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	if p.fn != nil {
		p.fn(p.written, p.total)
	}
	return len(b), nil
}

// RebindRefsOptions contains options for rebinding multiple refs.
type RebindRefsOptions struct {
	SrcRepo  string
	DestRepo string
	Refs     []repoinfo.Info
	Executor executil.Executor
}

// RebindRefs copies refs from SrcRepo to DestRepo, initializing DestRepo if it is missing.
func RebindRefs(opts RebindRefsOptions) error {
	if opts.Executor == nil {
		opts.Executor = executil.NewOSExecutor()
	}
	if err := os.MkdirAll(opts.DestRepo, 0755); err != nil {
		return fmt.Errorf("failed to create target repo directory: %w", err)
	}
	if _, err := os.Stat(filepath.Join(opts.DestRepo, "config")); os.IsNotExist(err) {
		logger.Debug("Initializing target OSTree repo at: %s", opts.DestRepo)
		targetInitCmd := opts.Executor.Command("ostree", "--repo="+opts.DestRepo, "init", "--mode=archive-z2")
		var initStderr bytes.Buffer
		targetInitCmd.SetStderr(&initStderr)
		if err := targetInitCmd.Run(); err != nil {
			return fmt.Errorf("failed to initialize target ostree repo (%w): %s", err, initStderr.String())
		}
	}

	for _, ra := range opts.Refs {
		destRef := ra.Ref()
		logger.Info("Copying ref %s from temp repo to target repo...", destRef)
		copyCmd := opts.Executor.Command("flatpak", "build-commit-from",
			"--src-repo="+opts.SrcRepo,
			"--src-ref="+destRef,
			"--update-appstream",
			"--no-update-summary",
			opts.DestRepo,
			destRef,
		)
		var copyStderr bytes.Buffer
		copyCmd.SetStderr(&copyStderr)
		if err := copyCmd.Run(); err != nil {
			return fmt.Errorf("failed to copy commit to target repository (%w): %s", err, copyStderr.String())
		}
	}
	return nil
}
