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

The CLI parses settings from a configuration file, looking for `aetherpak.yaml` or `aetherpak.yml` in the local working directory by default.

> [!NOTE]
> The configuration file can use both `.yaml` and `.yml` extensions. You can also specify a custom configuration file path at runtime using the `--config` flag or override any configuration parameter using environment variables prefixed with `AETHERPAK_` (e.g. `AETHERPAK_REGISTRY`).

### Configuration Schema

#### Global Settings
* **`registry`** (string): The target OCI registry host (e.g., `ghcr.io` or `quay.io`).
* **`pages_url`** (string): The public URL where the repository landing page and index files are hosted.
* **`remote_name`** (string): The repository name configured in user Flatpak clients (defaults to `<owner>-<repo>`).
* **`signing_mode`** (string): The signature verification strategy. Supported values: `auto` (sign if keys are present), `gpg` (enforce GPG signing), or `off` (default).
* **`repo_title`** (string): Customized title shown on the landing page and `.flatpakrepo` metadata (defaults to `"Flatpak Repository"`).
* **`repo_homepage`** (string): URL link for repository homepage metadata.
* **`runtime_repo`** (string): Fallback `.flatpakrepo` URL used to resolve dependencies (defaults to Flathub).
* **`channel_mappings`** (map[string]string): Key-value pairs mapping Git references (supporting glob wildcards like `staging/*`) to target flatpak branches.

#### `branding`
Customizes the look and feel of the generated landing page:
* **`logo_url`** (string): URL to a custom repository header logo.
* **`favicon_url`** (string): URL to a page favicon file.
* **`accent_color`** (string): Hex color code defining the primary brand accent (defaults to `#8b5cf6`).
* **`footer_text`** (string): Custom text/HTML to display in the footer (defaults to `"Powered by AetherPak"`).
* **`index_template`** (string): Local path to an alternative HTML file template to override index generation entirely.

#### `linter`
Global linter behavior configuration:
* **`strict`** (boolean): Set to `true` to fail builds if any linter warnings or errors are raised.
* **`ignore_rules`** (list[string]): Specific `flatpak-builder-lint` rule IDs to bypass.

#### `defaults`
Fallback build configurations applied when individual application settings are omitted:
* **`ccache`** (boolean): Enable compiler cache to speed up compilation.
* **`ccache_dir`** (string): Custom folder directory to store compiler cache assets.
* **`state_dir`** (string): Path to store intermediate state outputs (defaults to `.state`).
* **`run_linter`** (boolean): Set to `true` to run linter checks on manifests and built repositories.
* **`builder_args`** (list[string]): Additional command-line flags to pass directly to `flatpak-builder`.

#### `apps`
A list of applications managed in the repository. Each entry supports the following settings:
* **`id`** (string, required): The reverse-DNS Flatpak application identifier (e.g. `org.example.App`).
* **`branch`** (string): The release channel branch (defaults to `stable`).
* **`arches`** (list[string]): Target architectures to compile/import (defaults to `[x86_64]`).
* **`manifest`** (string): Local relative path to the Flatpak manifest file (required for source-based builds).
* **`runtime`** (string): Upstream runtime dependencies list (required for source-based builds).
* **`run-linter`** (boolean): Local toggle to execute linter validation checks.
* **`linter`** (block): Override block for linter strictness and exceptions.
* **`ccache`** / **`ccache_dir`** / **`state_dir`** / **`builder_args`**: Application-specific overrides for compilation parameters.
* **`bundles`** (map[string]Bundle): Prebuilt Flatpak bundle inputs mapped per architecture. Under each arch (e.g. `x86_64`):
  * **`url`** (string, required): Download link to the `.flatpak` bundle.
  * **`sha256`** (string, required): 64-character SHA-256 validation checksum of the file.

---

### Example Configuration (`aetherpak.yaml`)

```yaml
registry: ghcr.io
pages_url: https://flatpak.example.com
remote_name: example-repo
repo_title: "My Custom Flatpak Repository"

channel_mappings:
  "main": "beta"
  "staging/*": "alpha"

linter:
  strict: true
  ignore_rules: ["appstream-screenshot-missing"]

defaults:
  ccache: true
  run_linter: true
  state_dir: ".builder-state"
  builder_args: ["--sandbox", "--disable-rofiles-fuse"]

branding:
  logo_url: "https://example.com/logo.png"
  accent_color: "#a855f7"
  footer_text: "Custom Repo Landing Page Footer"

apps:
  - id: org.example.App
    manifest: apps/org.example.App/manifest.json
    runtime: gnome-50
    arches: [x86_64, aarch64]
    run-linter: true

  - id: com.example.Other
    branch: beta
    bundles:
      x86_64:
        url: https://upstream.com/Other_x86_64.flatpak
        sha256: 2159fc643175dcf54f8b9293f48fb8b11577fa0ea5514ea47d4e3ef4431f13b1
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
Resolves the flatpak channel name from git ref metadata:
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

Guidelines on compilation setup, local prerequisites, test harness drivers, and coding styles are documented in [CONTRIBUTING.md](CONTRIBUTING.md).

