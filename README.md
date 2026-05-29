# AetherPak Core CLI

A standalone, performance-focused Go command-line tool for orchestrating Flatpak application build pipelines, registry distribution (OCI), and static index site generation.

---

## Architecture

The CLI follows a **Plumbing vs. Porcelain** design:
* **Plumbing (Primitives):** Independent, highly-scoped, single-responsibility commands suited for complex workflows or customizable matrices.
* **Porcelain (Convenience Wrappers):** Standard high-level triggers that coordinate plumbing calls automatically in-memory.

For details on the Go package layout:
* [`pkg/config`](pkg/config/): Configuration parsing and validations rules.
* [`pkg/record`](pkg/record/): Execution output records JSON contracts (`record.json` / `labels.json`).
* [`pkg/plan`](pkg/plan/): Planning engine and git diff matrices logic.
* [`pkg/builder`](pkg/builder/): system wrapper for `flatpak-builder`.
* [`pkg/importer`](pkg/importer/): bundle downloader, checker, and rebind.
* [`pkg/oci`](pkg/oci/): OSTree-to-OCI tags compiler and push transporters.
* [`pkg/signing`](pkg/signing/): cryptographical in-memory detached GPG signatures.
* [`pkg/site`](pkg/site/): indexing merges and site aggregation engines.
---

## Configuration

The CLI parses settings from an optional `aetherpak.yaml` (or `aetherpak.yml`) file. All settings can be overridden at runtime using environment variables prefixed with `AETHERPAK_` (e.g. `AETHERPAK_REGISTRY` or `AETHERPAK_DEFAULTS_RUN_LINTER`).

```yaml
registry: ghcr.io
pages_url: https://flatpak.example.com
remote_name: example-repo
repo_title: "My Custom Flatpak Repository"

# Channel mappings supporting exact and wildcard matching
channel_mappings:
  "main": "beta"
  "staging/*": "alpha"

# Global linter options
linter:
  strict: true
  ignore_rules: ["appstream-screenshot-missing"]

# Build defaults
defaults:
  ccache: true
  run_linter: true
  state_dir: ".builder-state"

# HTML landing page customization
branding:
  logo_url: "https://example.com/logo.png"
  accent_color: "#a855f7"
  footer_text: "Custom Repo Landing Page Footer"
  index_template: "templates/custom_index.html" # Path to custom repository index HTML template

apps:
  - id: org.example.App
    manifest: apps/org.example.App/manifest.json
    runtime: gnome-50
```

---

## Command Reference

### Root Options

* `--config <path>` (optional path to `aetherpak.yaml`, defaults to check for `aetherpak.yaml` locally).
* `-v, --verbose` (enable verbose debugging statements).
* `--json-log` (enable JSON formatted structured output logs).
* `--plain` (disable colors, emojis, and fancy formatting; plain text output).
* `--no-color` (alias for `--plain` to disable colors and fancy formatting).

### Plumbing Commands

#### `plan`
Computes matrices for changed assets since a specific base SHA diff:
```bash
aetherpak plan --base-sha <sha> --workflow-path <path> --output json
```

#### `build`
Wraps `flatpak-builder` sandbox compilation:
```bash
aetherpak build --app org.example.App --manifest apps/manifest.json --arch x86_64
```

#### `import`
Ingests prebuilt bundles (`.flatpak`) and rebinds channels:
```bash
aetherpak import --app org.example.App --bundle-url https://... --bundle-sha256 <hex>
```

#### `push-oci`
Converts repo branch to OCI image layer and pushes:
```bash
aetherpak push-oci --app org.example.App --registry ghcr.io --oci-repository my-org/my-app
```

#### `build-site`
Downloads old static index, merges recent cell records, and regenerates index listings:
```bash
aetherpak build-site --pages-url https://flatpak.my-org.com --site-dir _site --reconcile --index-template templates/custom_index.html
```

#### `resolve-channel`
Resolves the flatpak channel name from git ref metadata (supports GitHub Actions, GitLab CI, and AetherPak env overrides):
```bash
aetherpak resolve-channel --ref-type tag --ref-name v1.0.0
```

#### `inspect-repo`
Resolves the app-id, arch, and branch channel from an existing OSTree repository metadata:
```bash
aetherpak inspect-repo --repo-path repo
```

### Porcelain Commands

#### `publish`
Chains compilation/importer and OCI push sequentially in-memory for a single target application:
```bash
aetherpak publish --app org.example.App --registry ghcr.io
```

#### `release`
Coordinates the entire lifecycle: runs matrix planner, compiles/imports changed records concurrently, pushes artifacts, and builds site index layouts:
```bash
aetherpak release --base-sha <sha> --workers 4 --index-template templates/custom_index.html
```

---

## Development

### Requirements
* Go 1.26+
* `flatpak`, `ostree` (for local system execution)
* `gpg` (required for signing and running integration tests)
* `flatpak-builder-lint` (required if running build commands with `--run-linter` active on non-containerized runners)

### Build
Compile the binary:
```bash
make build
```
The output binary will be written to `bin/aetherpak`.

### Test
Run unit tests:
```bash
make test
```
Run E2E integration tests (requires docker/podman and compose):
```bash
make test/integration
```

### Quality & Formatting
Format the codebase:
```bash
make fmt
```
Vet the codebase:
```bash
make vet
```
