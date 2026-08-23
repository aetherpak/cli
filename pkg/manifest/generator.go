package manifest

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/config"
	"gopkg.in/yaml.v3"
)

// GenerateOptions contains parameters for generating a standard Flatpak manifest.
type GenerateOptions struct {
	AppID          string
	Runtime        string
	RuntimeVersion string
	SDK            string
	SDKVersion     string
	Command        string
	FinishArgs     []string
	Binaries       []config.BinarySource
	Desktop        string
	Metainfo       string
	Icons          string
	Files          []config.FileSource
	Symlinks       []string
	BuildCommands  []string
	PostInstall    []string
}

func computeRelPath(baseDir, hostPath string) string {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		absBase = baseDir
	}
	absHost, err := filepath.Abs(hostPath)
	if err != nil {
		absHost = hostPath
	}
	rel, err := filepath.Rel(absBase, absHost)
	if err != nil {
		return hostPath
	}
	return rel
}

func detectIconSize(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return "", false
	}
	if cfg.Width > 0 && cfg.Height > 0 && cfg.Width == cfg.Height {
		return fmt.Sprintf("%dx%d", cfg.Width, cfg.Height), true
	}
	return "", false
}

// GenerateManifestYAML generates a standard Flatpak manifest YAML from GenerateOptions.
// baseDir is the directory where the generated manifest file will reside (used to calculate relative paths to sources).
func GenerateManifestYAML(opts GenerateOptions, baseDir string) ([]byte, error) {
	if opts.AppID == "" {
		return nil, fmt.Errorf("application ID is required")
	}
	if opts.Runtime == "" {
		return nil, fmt.Errorf("runtime is required")
	}

	sdk := opts.SDK
	if sdk == "" {
		if strings.Contains(opts.Runtime, "Platform") {
			sdk = strings.Replace(opts.Runtime, "Platform", "Sdk", 1)
		} else {
			sdk = "org.freedesktop.Sdk"
		}
	}

	command := opts.Command
	if command == "" {
		if len(opts.Binaries) > 0 {
			dest := opts.Binaries[0].Dest
			if dest == "" {
				dest = opts.Binaries[0].Path
			}
			command = filepath.Base(dest)
		} else {
			parts := strings.Split(opts.AppID, ".")
			command = parts[len(parts)-1]
		}
	}

	finishArgs := opts.FinishArgs
	if len(finishArgs) == 0 {
		finishArgs = []string{
			"--socket=wayland",
			"--socket=fallback-x11",
			"--share=ipc",
			"--share=network",
			"--device=dri",
		}
	}

	var buildCommands []string
	var sourcesList []map[string]interface{}
	stagedNames := make(map[string]int)

	getUniqueStagedName := func(baseName string) string {
		count, exists := stagedNames[baseName]
		if !exists {
			stagedNames[baseName] = 1
			return baseName
		}
		stagedNames[baseName] = count + 1
		ext := filepath.Ext(baseName)
		noExt := strings.TrimSuffix(baseName, ext)
		return fmt.Sprintf("%s_%d%s", noExt, count+1, ext)
	}

	// 1. Process Binaries
	for _, b := range opts.Binaries {
		hostPath := b.Path
		if hostPath == "" {
			hostPath = b.Src
		}
		if hostPath == "" {
			continue
		}

		relPath := computeRelPath(baseDir, hostPath)
		sandboxName := getUniqueStagedName(filepath.Base(hostPath))
		dest := b.Dest
		if dest == "" {
			dest = "/app/bin/" + filepath.Base(hostPath)
		}

		sourcesList = append(sourcesList, map[string]interface{}{
			"type":          "file",
			"path":          relPath,
			"dest-filename": sandboxName,
		})

		buildCommands = append(buildCommands, fmt.Sprintf("install -Dm755 %q %q", sandboxName, dest))
	}

	// 2. Process Desktop file
	if opts.Desktop != "" {
		hostPath := opts.Desktop
		relPath := computeRelPath(baseDir, hostPath)
		sandboxName := getUniqueStagedName(filepath.Base(hostPath))
		dest := fmt.Sprintf("/app/share/applications/%s.desktop", opts.AppID)

		sourcesList = append(sourcesList, map[string]interface{}{
			"type":          "file",
			"path":          relPath,
			"dest-filename": sandboxName,
		})

		buildCommands = append(buildCommands, fmt.Sprintf("install -Dm644 %q %q", sandboxName, dest))
		buildCommands = append(buildCommands, fmt.Sprintf("if command -v desktop-file-edit >/dev/null 2>&1; then desktop-file-edit --set-icon=%q %q || true; fi", opts.AppID, dest))
		buildCommands = append(buildCommands, fmt.Sprintf("sed -i -E 's|^Exec=/usr/(local/)?bin/|Exec=|' %q || true", dest))
	}

	// 3. Process Metainfo file
	if opts.Metainfo != "" {
		hostPath := opts.Metainfo
		relPath := computeRelPath(baseDir, hostPath)
		sandboxName := getUniqueStagedName(filepath.Base(hostPath))
		dest := fmt.Sprintf("/app/share/metainfo/%s.metainfo.xml", opts.AppID)

		sourcesList = append(sourcesList, map[string]interface{}{
			"type":          "file",
			"path":          relPath,
			"dest-filename": sandboxName,
		})

		buildCommands = append(buildCommands, fmt.Sprintf("install -Dm644 %q %q", sandboxName, dest))
	}

	// 4. Process Icons
	if opts.Icons != "" {
		hostPath := opts.Icons
		relPath := computeRelPath(baseDir, hostPath)

		isDir := false
		if fi, err := os.Stat(hostPath); err == nil && fi.IsDir() {
			isDir = true
		} else if strings.HasSuffix(hostPath, "/") {
			isDir = true
		}

		if isDir {
			sandboxDir := getUniqueStagedName(filepath.Base(strings.TrimSuffix(hostPath, "/")))
			sourcesList = append(sourcesList, map[string]interface{}{
				"type": "dir",
				"path": relPath,
				"dest": sandboxDir,
			})

			// Inspect directory if accessible on host
			var discoveredCommands []string
			if entries, err := os.ReadDir(hostPath); err == nil && len(entries) > 0 {
				for _, e := range entries {
					if e.IsDir() {
						continue
					}
					name := e.Name()
					lowerName := strings.ToLower(name)
					stagedPath := fmt.Sprintf("%s/%s", sandboxDir, name)
					if strings.HasSuffix(lowerName, ".svg") {
						dest := fmt.Sprintf("/app/share/icons/hicolor/scalable/apps/%s.svg", opts.AppID)
						discoveredCommands = append(discoveredCommands, fmt.Sprintf("install -Dm644 %q %q", stagedPath, dest))
					} else if strings.HasSuffix(lowerName, ".png") {
						baseNoExt := strings.TrimSuffix(name, filepath.Ext(name))
						switch baseNoExt {
						case "16x16", "32x32", "48x48", "64x64", "128x128", "256x256", "512x512":
							dest := fmt.Sprintf("/app/share/icons/hicolor/%s/apps/%s.png", baseNoExt, opts.AppID)
							discoveredCommands = append(discoveredCommands, fmt.Sprintf("install -Dm644 %q %q", stagedPath, dest))
						case "128x128@2x":
							dest := fmt.Sprintf("/app/share/icons/hicolor/256x256/apps/%s.png", opts.AppID)
							discoveredCommands = append(discoveredCommands, fmt.Sprintf("install -Dm644 %q %q", stagedPath, dest))
						case "icon", opts.AppID:
							dest := fmt.Sprintf("/app/share/icons/hicolor/512x512/apps/%s.png", opts.AppID)
							discoveredCommands = append(discoveredCommands, fmt.Sprintf("install -Dm644 %q %q", stagedPath, dest))
						}
					}
				}
			}

			if len(discoveredCommands) > 0 {
				buildCommands = append(buildCommands, discoveredCommands...)
			} else {
				// Fallback shell commands inside sandbox
				buildCommands = append(buildCommands,
					fmt.Sprintf("for sz in 16x16 32x32 48x48 64x64 128x128 256x256 512x512; do if [ -f %[1]q/${sz}.png ]; then install -Dm644 %[1]q/${sz}.png \"/app/share/icons/hicolor/${sz}/apps/%[2]s.png\"; fi; done", sandboxDir, opts.AppID),
					fmt.Sprintf("if [ -f %[1]q/icon.png ]; then install -Dm644 %[1]q/icon.png \"/app/share/icons/hicolor/512x512/apps/%[2]s.png\"; fi", sandboxDir, opts.AppID),
					fmt.Sprintf("if [ -f %[1]q/icon.svg ]; then install -Dm644 %[1]q/icon.svg \"/app/share/icons/hicolor/scalable/apps/%[2]s.svg\"; fi", sandboxDir, opts.AppID),
				)
			}
		} else {
			sandboxName := getUniqueStagedName(filepath.Base(hostPath))
			sourcesList = append(sourcesList, map[string]interface{}{
				"type":          "file",
				"path":          relPath,
				"dest-filename": sandboxName,
			})
			lowerName := strings.ToLower(hostPath)
			if strings.HasSuffix(lowerName, ".svg") {
				buildCommands = append(buildCommands, fmt.Sprintf("install -Dm644 %q %q", sandboxName, fmt.Sprintf("/app/share/icons/hicolor/scalable/apps/%s.svg", opts.AppID)))
			} else {
				iconSize := "512x512"
				if sz, ok := detectIconSize(hostPath); ok {
					iconSize = sz
				} else {
					baseNoExt := strings.TrimSuffix(sandboxName, filepath.Ext(sandboxName))
					switch baseNoExt {
					case "16x16", "32x32", "48x48", "64x64", "128x128", "256x256", "512x512":
						iconSize = baseNoExt
					case "128x128@2x":
						iconSize = "256x256"
					}
				}
				dest := fmt.Sprintf("/app/share/icons/hicolor/%s/apps/%s.png", iconSize, opts.AppID)
				buildCommands = append(buildCommands, fmt.Sprintf("install -Dm644 %q %q", sandboxName, dest))
				baseNoExt := strings.TrimSuffix(sandboxName, filepath.Ext(sandboxName))
				if baseNoExt != opts.AppID && baseNoExt != "" && !strings.Contains(baseNoExt, "x") {
					aliasDest := fmt.Sprintf("/app/share/icons/hicolor/%s/apps/%s.png", iconSize, baseNoExt)
					buildCommands = append(buildCommands, fmt.Sprintf("ln -sf %q %q", opts.AppID+".png", aliasDest))
				}
			}
		}
	}

	// 5. Process Generic Files
	for _, f := range opts.Files {
		hostPath := f.Path
		if hostPath == "" {
			hostPath = f.Src
		}
		if hostPath == "" {
			continue
		}

		relPath := computeRelPath(baseDir, hostPath)

		isDir := false
		if fi, err := os.Stat(hostPath); err == nil && fi.IsDir() {
			isDir = true
		} else if strings.HasSuffix(hostPath, "/") {
			isDir = true
		}

		if isDir {
			sandboxDir := getUniqueStagedName(filepath.Base(strings.TrimSuffix(hostPath, "/")))
			sourcesList = append(sourcesList, map[string]interface{}{
				"type": "dir",
				"path": relPath,
				"dest": sandboxDir,
			})
			buildCommands = append(buildCommands, fmt.Sprintf("install -d %q && cp -r %q/. %q", f.Dest, sandboxDir, f.Dest))
		} else {
			sandboxName := getUniqueStagedName(filepath.Base(hostPath))
			sourcesList = append(sourcesList, map[string]interface{}{
				"type":          "file",
				"path":          relPath,
				"dest-filename": sandboxName,
			})
			buildCommands = append(buildCommands, fmt.Sprintf("install -Dm644 %q %q", sandboxName, f.Dest))
		}
	}

	// 6. Process Symlinks
	for _, s := range opts.Symlinks {
		var src, dest string
		if idx := strings.Index(s, ":"); idx > 0 {
			src = strings.TrimSpace(s[:idx])
			dest = strings.TrimSpace(s[idx+1:])
		} else {
			fields := strings.Fields(s)
			if len(fields) == 2 {
				src = fields[0]
				dest = fields[1]
			}
		}
		if src != "" && dest != "" {
			buildCommands = append(buildCommands, fmt.Sprintf("ln -sf %q %q", src, dest))
		}
	}

	// 7. Process BuildCommands & PostInstall
	if len(opts.BuildCommands) > 0 {
		buildCommands = append(buildCommands, opts.BuildCommands...)
	}
	if len(opts.PostInstall) > 0 {
		buildCommands = append(buildCommands, opts.PostInstall...)
	}

	// Assemble Manifest Document
	manifestDoc := map[string]interface{}{
		"id":              opts.AppID,
		"runtime":         opts.Runtime,
		"runtime-version": opts.RuntimeVersion,
		"sdk":             sdk,
		"command":         command,
		"finish-args":     finishArgs,
		"modules": []map[string]interface{}{
			{
				"name":           opts.AppID,
				"buildsystem":    "simple",
				"build-commands": buildCommands,
				"sources":        sourcesList,
			},
		},
	}

	if opts.SDKVersion != "" {
		manifestDoc["sdk-version"] = opts.SDKVersion
	}

	return yaml.Marshal(manifestDoc)
}
