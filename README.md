# AetherPak Core CLI

A standalone, performance-focused Go command-line tool for orchestrating Flatpak application build pipelines, registry distribution (OCI), and static index site generation.

---

## Installation

AetherPak CLI is a single compiled Go binary. It has no runtime dependencies of its own, but the commands that compile, sign, and lint Flatpak applications expect the usual Flatpak toolchain on the host.

### System Prerequisites

* **flatpak** and **ostree**: required for repository, bundle, and ref operations.
* **flatpak-builder**: required by `build`, `publish`, and `release` when compiling from a manifest.
* **flatpak-builder-lint**: optional, used when `run_linter` is enabled.

Signing does not shell out to `gpg`. Repositories and OCI images are signed in-process from armored GPG key material, so no `gpg` binary is needed. Disable signing with `no_sign: true` or `--no-sign`.

Run `aetherpak status` after installing to see which of these are available.

### Option 1: Prebuilt Binary

Download the release archive for your architecture, extract it, and install the binary onto your `PATH`:

```bash
# x86_64
curl -LO https://github.com/aetherpak/cli/releases/latest/download/aetherpak-linux-amd64.tar.gz
tar -xzf aetherpak-linux-amd64.tar.gz
sudo install -Dm755 aetherpak-linux-amd64 /usr/local/bin/aetherpak

# aarch64
curl -LO https://github.com/aetherpak/cli/releases/latest/download/aetherpak-linux-arm64.tar.gz
tar -xzf aetherpak-linux-arm64.tar.gz
sudo install -Dm755 aetherpak-linux-arm64 /usr/local/bin/aetherpak
```

### Option 2: Build from Source

With Go 1.26.3 or newer, clone the repository and install the binary from the checkout:

```bash
git clone https://github.com/aetherpak/cli.git
cd cli
go install .
```

`go install .` writes the binary to `$(go env GOPATH)/bin/aetherpak`, so make sure that directory is on your `PATH`. A source build reports its version as `dev`, since the release version is stamped in at build time. For a version-stamped local build, `make build` writes the binary to `bin/aetherpak`.

### Option 3: Container Image

The published CLI image bakes in the Flatpak toolchain, so it needs no host setup. Mount the repository you want to build into `/workspace`:

```bash
# CLI image
podman run --rm -v "$PWD:/workspace" ghcr.io/aetherpak/cli:latest aetherpak status

# Builder image, with flatpak-builder available
podman run --rm -v "$PWD:/workspace" ghcr.io/aetherpak/cli:latest-builder aetherpak build --app-id org.example.App
```

Replace `podman` with `docker` if you use that runtime.

### Verify the Installation

```bash
aetherpak --version
aetherpak status
```

`status` prints a per-dependency report and validates your configuration file and GPG keys.

---

## Quick Start

AetherPak reads repository settings from `aetherpak.yaml` (or `aetherpak.yml`) in the working directory. A minimal single-app configuration looks like:

```yaml
app_id: org.example.App
runtime: org.freedesktop.Platform//25.08
manifest: apps/org.example.App/manifest.yaml
```

Then walk through a first build:

```bash
# Inspect the resolved configuration and active overrides
aetherpak config show

# Validate the local toolchain, config file, and signing setup
aetherpak status

# Build, push, and sign a single app end to end
aetherpak publish --app-id org.example.App --registry ghcr.io

# Or plan and release every app changed since a base commit
aetherpak release --base-sha <git-sha>
```

If you would rather not write the configuration by hand, `aetherpak add` can bootstrap it from a local manifest, a bundle URL, or a git repository:

```bash
aetherpak add --manifest org.example.App.yaml
```

The full command surface, including the plumbing primitives and configuration schema, is documented under [Command Reference](#command-reference) below.

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
* **`oci_repository`** (string): The target repository path/name for OCI distribution (replaces deprecated `remote_name` for OCI registry pushes).
* **`remote_name`** (string): The repository name configured in user Flatpak clients (defaults to `<owner>-<repo>`). Historically used for OCI registry pushes; now acts as a fallback for `oci_repository` (deprecated for registry pushes).
* **`output_dir`** (string): Base directory for all output assets (state, records, site, ccache, repo) unless overridden.
* **`no_sign`** (boolean): Set to `true` to disable GPG signing of repositories and OCI images entirely (defaults to `false`).
* **`image_deltas`** (boolean): Set to `true` to publish OCI image deltas, so clients on a previous release can update by downloading only the changed layer data (defaults to `false`). See [Image Deltas](#image-deltas).
* **`delta_back`** (integer): How many previous releases to diff against when image deltas are enabled (1-10, defaults to `1`; values below 1 are treated as 1).
* **`repo_title`** (string): Customized title shown on the landing page and `.flatpakrepo` metadata (defaults to the value of `remote_name`, which defaults to `"aetherpak"`).
* **`repo_homepage`** (string): URL link for repository homepage metadata.
* **`runtime_repo`** (string): Fallback `.flatpakrepo` URL used to resolve dependencies (defaults to Flathub).
* **`channel_mappings`** (map[string]string): Key-value pairs mapping Git references (supporting glob wildcards like `staging/*`) to target flatpak branches.

#### `branding`
Customizes the look and feel of the generated landing page:
* **`logo_url`** (string): URL to a custom repository header logo.
* **`favicon_url`** (string): URL to a page favicon file.
* **`accent_color`** (string): Hex color code defining the primary brand accent (defaults to `#8b5cf6`).
* **`footer_text`** (string): Custom text/HTML to display in the footer (defaults to `"Powered by AetherPak"`).
* **`index_template`** (string): Local path to an alternative HTML file template to override index generation entirely. Supports structured Go HTML templates with access to repository applications and formatting helper functions (see [Custom Index Templating](#custom-index-templating) below).

#### `linter`
Global linter behavior configuration:
* **`strict`** (boolean): Set to `true` to fail builds if any linter warnings or errors are raised.
* **`ignore_rules`** (list[string]): Specific `flatpak-builder-lint` rule IDs to bypass.
* **`exceptions`** (list[string]): Inline list of `flatpak-builder-lint` rule IDs to bypass (alternative/alias for `ignore_rules`). Can be overridden via the `AETHERPAK_LINTER_EXCEPTIONS` environment variable or the `--linter-exception` flag.
* **`exceptions_file`** (string): Local path to a JSON configuration file containing app-specific or wildcard linter exceptions (e.g. `"linter-exceptions.json"`). Can be overridden via the `AETHERPAK_LINTER_EXCEPTIONS_FILE` environment variable or the `--linter-exceptions-file` flag.

#### `defaults`
Fallback build configurations applied when individual application settings are omitted:
* **`ccache`** (boolean): Enable compiler cache to speed up compilation.
* **`ccache_dir`** (string): Custom folder directory to store compiler cache assets.
* **`state_dir`** (string): Path to store intermediate state outputs (defaults to `.state`).
* **`run_linter`** (boolean): Set to `true` to run linter checks on manifests and built repositories.
* **`auto_release_metadata`** (boolean): Set to `true` to automatically stamp the current git tag/release version and date into AppStream metadata without modifying source files.
* **`image_deltas`** (boolean): Enable OCI image delta publishing for every app unless the app overrides it. Overrides the top-level value.
* **`delta_back`** (integer): Override the number of previous releases used as delta sources (1-10).
* **`builder_args`** (list[string]): Additional command-line flags to pass directly to `flatpak-builder`.
* **`no_install_deps`** (boolean): Disable automatic injection of `--install-deps-from` flags when running `flatpak-builder` (defaults to `false`).
* **`no_flathub`** (boolean): Disable automatic injection of the `flathub` remote as a dependency source (defaults to `false`).
* **`remotes`** (map[string]string | map[string]RemoteConfig): Map of Flatpak remote repository names to their configuration (pre-registered before build). Can be a simple URL string or an exploded map supporting:
  * **`url`** (string, required): The flatpakrepo URL.
  * **`gpg_verify`** (boolean, optional): Enable/disable GPG verification.
  * **`gpg_key`** (string, optional): GPG public key (can be a local path, URL, or inline ASCII-armored GPG public key block).
  * **`sig_verify_url`** (string, optional): Signature lookaside URL.
* **`flatpaks`** (list[FlatpakDep]): Flatpak runtimes or SDK extensions/dependencies to pre-install before build. Each entry requires a `remote` (string) and a `ref` (string).

##### `apps`
A list of applications managed in the repository. AetherPak supports three distinct packaging modes: **Zero-Manifest Mode**, **Traditional Manifest Mode**, and **Bundle Import Mode**.

Each entry in `apps` (or top-level single-app settings) supports:
* **`id`** / **`app_id`** (string, required): The reverse-DNS Flatpak application identifier (e.g. `org.example.App`).
* **`branch`** (string): The release channel branch (defaults to `stable`).
* **`arches`** (list[string]): Target architectures to compile/import (defaults to `[x86_64]`).
* **`runtime`** (string): Upstream runtime ref (e.g. `org.gnome.Platform//49`, `org.freedesktop.Platform//25.08`, `org.kde.Platform:6.7`).
* **`sdk`** (string, optional): Upstream SDK ID (automatically inferred from `runtime` if omitted, e.g. `org.gnome.Sdk`).
* **`command`** (string, optional): Primary executable name (automatically inferred from primary binary if omitted).
* **`finish_args`** / **`finish-args`** (list[string], optional): Sandboxing permissions passed to Flatpak (defaults to standard GUI sandbox arguments if omitted).
* **`sources`** (block): Direct binary and asset assembly declarations for **Zero-Manifest Mode** (see below).
* **`manifest`** (string): Local relative path to the Flatpak manifest file (used for **Traditional Manifest Mode**).
* **`bundles`** (map[string]Bundle): Prebuilt Flatpak bundle inputs mapped per architecture (used for **Bundle Import Mode**). Under each arch (e.g. `x86_64`):
  * **`url`** (string, required): Download link to the `.flatpak` bundle.
  * **`sha256`** (string, required): 64-character SHA-256 validation checksum of the file.
* **`run-linter`** (boolean): Local toggle to execute linter validation checks.
* **`auto_release_metadata`** (boolean): Application-specific toggle to enable automatic AppStream release metadata synchronization.
* **`image_deltas`** (boolean): Application-specific toggle for OCI image delta publishing, overriding `defaults.image_deltas`.
* **`delta_back`** (integer): Application-specific number of previous releases used as delta sources, overriding `defaults.delta_back` (1-10).
* **`linter`** (block): Override block for linter strictness, ignore rules, and exceptions. Supports `strict` (boolean), `ignore_rules` (list[string]), `exceptions` (list[string]), and `exceptions_file` (string).
* **`ccache`** / **`ccache_dir`** / **`state_dir`** / **`builder_args`** / **`no_install_deps`** / **`no_flathub`**: Application-specific overrides for compilation parameters.
* **`remotes`** / **`flatpaks`**: Application-specific overrides/merges for Flatpak remotes and dependencies.

---

### Image Deltas

AetherPak can publish [Flatpak OCI image deltas](https://github.com/flatpak/flatpak-oci-specs/blob/main/image-deltas.md) alongside each image. Clients that already have the previous release installed then download a binary delta instead of the whole layer.

Enable it globally, under `defaults`, or per app:

```yaml
image_deltas: true
```

`aetherpak push-oci` also accepts `--image-deltas`, `--no-image-deltas`, and `--delta-back`; the flags override configuration.

How it behaves:

* The previous image's concrete manifest digest is resolved before the app/branch/arch tag is updated, so the delta still works on registries that drop untagged indexes; the diff and upload happen after the new image and its record are written. The first push therefore has no delta.
* `delta_back` controls how many previous releases become delta sources (1-10). The default of `1` covers clients on the immediately previous release; higher values add up to one delta layer per earlier release. A source whose diff would not be smaller, or whose history was not recorded by a previous delta-enabled release, is omitted. Each extra source costs one layer download, one diff, and its retained delta blob.
* Deltas are published to the same OCI repository under the standard `_deltaindex` tag. Each delta manifest is kept under a `_delta-<target-digest>` tag, and each source image under a `_deltasrc-<digest>` tag, so both survive registry garbage collection and the index can be rebuilt from the tag set. Flatpak verifies the rebuilt layer against the diffID declared in the target image's signed manifest; deltas do not need a separate signature.
* Delta generation is best-effort at publish time. If the previous image cannot be read, the layers are not single-layer, the diff would not be smaller than the full layer, or the delta cannot be published, the image push still succeeds and no delta is advertised. Once Flatpak has selected a published delta, a verification failure fails that pull; it does not silently retry the full layer.
* Push jobs get slower and the registry stores the delta blobs, because each previous layer is downloaded and diffed. `_deltaindex`, `_delta-*`, and `_deltasrc-*` grow with releases; prune them with `aetherpak deltas prune`. Leave `image_deltas` off if that trade-off does not suit the project.
* Current limitation: only single-layer images are supported. Multi-layer images are skipped. Extension refs (for example `.Locale`) are not listed as apps and therefore inherit `defaults`/global rather than the base app's value.
* `_deltaindex` is eventually consistent during a parallel release. If a delta publish is interrupted, the delta can be missing for that release and is repaired by a later push; the image and its record are written before any delta work starts.
* Prune stale tags with `aetherpak deltas prune --dry-run`, then without `--dry-run`. It removes `_delta-*` tags whose target image is gone, and `_deltasrc-*` tags no surviving delta or application tag references, then rebuilds `_deltaindex` from the remaining tags. Auth, network, and other transient errors leave tags untouched. It needs registry write credentials (`--dry-run` only needs read access) and should run as a serialized maintenance step, not from parallel push jobs.

Requirements: a Flatpak build with OCI remote support and zstd (validated with Flatpak 1.18.x). Older clients, or clients on releases older than the configured `delta_back` window, simply download the full layer.

---

### Packaging Modes: Choosing the Right Mode

| Mode | When to Choose | Key Characteristics |
| :--- | :--- | :--- |
| **Zero-Manifest Mode** | **Recommended for prebuilt binaries** (Rust, Go, C++, Tauri, Electron, or extracted `.deb`/tarball artifacts). | Declares binaries, desktop files, icons, and metainfo directly in `aetherpak.yaml`. No external Flatpak manifest, staging shell scripts, or manual `install -Dm755` rules needed. Automatic icon dimension detection and deep CI matrix change tracking. |
| **Traditional Manifest Mode** | **For building apps from source** with multi-module dependencies or complex build systems (Meson, Autotools, CMake, Cargo submodules). | References an existing `manifest.json` or `manifest.yaml` consumed directly by `flatpak-builder`. |
| **Bundle Import Mode** | **For distributing pre-existing `.flatpak` single-file bundles** already generated by upstream release pipelines. | Downloads, validates checksums, re-signs (optional), and publishes existing `.flatpak` bundles to your repository and OCI registry. |

---

### Zero-Manifest Mode (`sources`)

Zero-manifest mode eliminates the need to maintain separate Flatpak manifest YAML files and staging scripts. All source mappings support flexible, human-friendly syntaxes:

##### Supported Mapping Formats
For `binaries:`, `files:`, and `symlinks:`, AetherPak supports standard YAML lists and maps:
1. **List of `<source>: <dest>` Maps**:
   ```yaml
   binaries:
     - dist/con: /app/bin/con
     - dist/con-cli: /app/bin/con-cli
   ```
2. **Top-Level Dictionary Mapping (`<source>: <dest>`)**:
   ```yaml
   binaries:
     dist/con: /app/bin/con
     dist/con-cli: /app/bin/con-cli
   ```
3. **List of Bare Source Paths (Default Destination)**:
   ```yaml
   binaries:
     - dist/con        # automatically maps to /app/bin/con
     - dist/con-cli    # automatically maps to /app/bin/con-cli
   ```
4. **Structured Objects**:
   ```yaml
   binaries:
     - path: dist/con
       dest: /app/bin/con
   ```

#### Zero-Manifest Example (`aetherpak.yaml`)
```yaml
app_id: co.nowledge.con
runtime: org.freedesktop.Platform//25.08
sources:
  binaries:
    - dist/con-linux-x86_64/con: /app/bin/con
    - dist/con-linux-x86_64/con-cli: /app/bin/con-cli
  desktop: dist/co.nowledge.con.desktop
  metainfo: packaging/flatpak/co.nowledge.con.metainfo.xml
  icons: dist/con.png              # Auto-detects 512x512 / 256x256 / SVG dimensions
  files:
    - assets/themes: /app/share/themes
  symlinks:
    - /app/bin/con-cli: /app/bin/con-helper
```

---

### Multi-App Configuration Example (`aetherpak.yaml`)

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
  remotes:
    flathub: https://dl.flathub.org/repo/flathub.flatpakrepo
    custom_repo:
      url: https://example.com/repo.flatpakrepo
      gpg_verify: true
      gpg_key: https://example.com/keys/gpg.key
      sig_verify_url: https://example.com/signatures
  flatpaks:
    - remote: flathub
      ref: org.gnome.Sdk//45

branding:
  logo_url: "https://example.com/logo.png"
  accent_color: "#a855f7"
  footer_text: "Custom Repo Landing Page Footer"

apps:
  # 1. Zero-Manifest Mode (Precompiled Binary)
  - id: ai.lemonade_server.Lemonade
    runtime: org.gnome.Platform//49
    sources:
      binaries:
        - build/lemond: /app/bin/lemond
        - build/lemonade: /app/bin/lemonade
      desktop: data/lemonade-app.desktop
      metainfo: data/ai.lemonade_server.Lemonade.metainfo.xml
      icons: src/app/src-tauri/icons/

  # 2. Traditional Manifest Mode (Source Build)
  - id: org.example.App
    manifest: apps/org.example.App/manifest.json
    runtime: gnome-50
    arches: [x86_64, aarch64]
    run-linter: true

  # 3. Bundle Import Mode (Prebuilt Flatpak Bundle)
  - id: com.example.Other
    branch: beta
    bundles:
      x86_64:
        url: https://upstream.com/Other_x86_64.flatpak
        sha256: 2159fc643175dcf54f8b9293f48fb8b11577fa0ea5514ea47d4e3ef4431f13b1
```

---

### Linter Exceptions

AetherPak automatically registers default exceptions for linter checks that are not applicable to self-hosted/independent repositories:
- `appstream-external-screenshot-url`
- `appstream-screenshots-not-mirrored-in-ostree`

Exceptions can be configured in two ways:

#### 1. Inline Configuration
You can specify list of exceptions directly under the `linter.exceptions` property globally or per-app:
```yaml
linter:
  exceptions:
    - appstream-screenshot-missing
    - appstream-license-missing
```
Or override them at runtime using:
- CLI repeatable flag: `--linter-exception <rule>`
- Env Var: `AETHERPAK_LINTER_EXCEPTIONS` (comma-separated list of exceptions)

#### 2. External JSON File (Flathub format)
You can point to a linter exceptions JSON file containing app-specific or wildcard rules:
```json
{
  "org.example.App": [
    "appstream-screenshot-missing",
    "appstream-summary-too-long"
  ],
  "*": [
    "appstream-license-missing"
  ]
}
```
This file path can be configured globally/per-app in `aetherpak.yaml` using the `exceptions_file` property, or overridden at runtime via:
- CLI flag: `--linter-exceptions-file <path>`
- Env Var: `AETHERPAK_LINTER_EXCEPTIONS_FILE` (or `AETHERPAK_LINTER_EXCEPTIONS` if it has a `.json` suffix)

---

### Custom Index Templating

When using a custom template file via the `index_template` config option, the `--index-template` flag, or the `AETHERPAK_INDEX_TEMPLATE` environment variable, AetherPak executes the template using Go's `html/template` engine.

The template is executed with a structured context containing all resolved repository, branding, signing, and application details.

#### Template Context Structure

The data structure passed to your custom template is defined as follows:

* **`.RemoteName`** (string): The resolved Flatpak remote repository name.
* **`.RepoTitle`** (string): The repository title.
* **`.PagesURL`** (string): The URL where the repository static files are hosted.
* **`.RepoHomepage`** (string): The homepage URL link.
* **`.RuntimeRepo`** (string): The upstream runtime dependency repository URL.
* **`.LogoURL`** (string): Custom repository header logo URL.
* **`.LogoHTML`** (template.HTML): Pre-formatted HTML image element containing `LogoURL` if configured.
* **`.FaviconURL`** (string): Custom page favicon file URL.
* **`.AccentColor`** (string): Hex color code defining the brand accent color.
* **`.FooterText`** (template.HTML): Custom footer HTML text.
* **`.Signing`** (block):
  * **`.Signing.Enabled`** (boolean): True if GPG signing is enabled.
  * **`.Signing.Fingerprint`** (string): GPG public key fingerprint.
  * **`.Signing.PublicKey`** (string): Path to the armored public key file (`sigs/key.asc`).
  * **`.Signing.Lookaside`** (string): Path to the GPG signature lookaside directory (`sigs`).
* **`.Apps`** (list): A list of preprocessed, structured application records:
  * **`.ID`** (string): Reverse-DNS application identifier (e.g. `org.example.App`).
  * **`.Name`** (string): Application name extracted from AppStream appdata, falling back to ID.
  * **`.Summary`** (string): Application summary description extracted from AppStream appdata.
  * **`.Icon`** (string): URL to the 64x64 application icon if defined.
  * **`.Branches`** (list): List of release channel branches sorted newest first:
    * **`.Branch`** (string): Flatpak branch channel (e.g. `stable`, `beta`).
    * **`.Arches`** (list[string]): Alphabetic list of supported architectures (e.g. `[aarch64, x86_64]`).
    * **`.Timestamp`** (int64): Latest build release Unix epoch timestamp.
    * **`.FormattedDate`** (string): Human-readable release date formatted as `Jan 02, 2006`.
    * **`.InstalledSize`** (int64): Latest build installed size in bytes.
    * **`.DownloadSize`** (int64): Latest build download size in bytes.
    * **`.Commit`** (string): Flatpak commit identifier.
    * **`.RefFile`** (string): Path to target download flatpakref file (e.g. `refs/org.example.App-stable.flatpakref`).
    * **`.InstallCmd`** (string): Helper command to install the application branch client-side.

#### Template Helper Functions

You can use the following custom Go template helpers inside your custom template:

* **`join <slice> <separator>`**: Joins a slice of strings using the specified separator.
  * Example: `{{join .Arches "/"}}` -> `aarch64/x86_64`
* **`formatSize <bytes>`**: Formats raw bytes into a human-readable string representation.
  * Example: `{{formatSize .InstalledSize}}` -> `20 MB`
* **`formatDate <timestamp> <layout>`**: Formats a UNIX epoch timestamp using a standard Go time layout.
  * Example: `{{formatDate .Timestamp "2006-01-02"}}` -> `2026-06-01`

#### Example Custom Template

Here is a simple example of a custom HTML template:

```html
<!DOCTYPE html>
<html>
<head>
  <title>{{.RepoTitle}}</title>
  <link rel="icon" href="{{.FaviconURL}}">
</head>
<body>
  <h1>{{.RepoTitle}}</h1>

  {{range .Apps}}
    <div class="app-card">
      {{if .Icon}}<img src="{{.Icon}}" width="64">{{end}}
      <h2>{{.Name}} ({{.ID}})</h2>
      <p>{{.Summary}}</p>

      <h3>Releases</h3>
      <ul>
        {{range .Branches}}
          <li>
            Branch: <strong>{{.Branch}}</strong> | Arches: {{join .Arches ", "}}
            <br>Released on: {{.FormattedDate}} | Size: {{formatSize .InstalledSize}}
            <br>Install: <code>{{.InstallCmd}}</code>
          </li>
        {{end}}
      </ul>
    </div>
  {{end}}

  <footer>{{.FooterText}}</footer>
</body>
</html>
```

---

## Command Reference

### Root Options

* `--config <path>` (optional path to `aetherpak.yaml`, defaults to check for `aetherpak.yaml` locally).
* `--output-dir <path>` (base directory for all output assets unless overridden).
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
Options:
* `--override-branch <branch>`: Overrides the Flatpak branch (release channel) for all planned apps in the matrix.
* `--force <app-id>`: Rebuilds a specific app. Supports the Flatpak standard `app-id//branch` ref format (e.g. `org.example.App//beta`) to select that app and override its target branch.

#### `build`
Wraps `flatpak-builder` sandbox compilation:
```bash
aetherpak build --app-id org.example.App --manifest apps/manifest.json --arch x86_64 --linter-exception appstream-screenshot-missing --flatpak-remote flathub=https://dl.flathub.org/repo/flathub.flatpakrepo --flatpak-dep flathub:org.gnome.Sdk//45
```
Options:
* `--app-id <app-id>`: target application ID. Supports the Flatpak standard `app-id//branch` ref format (e.g. `org.example.App//beta`) to build that app and override its target branch.
* `--flatpak-remote <name>=<url>`: Repeatable flag to register Flatpak remotes before compiling.
* `--flatpak-dep <remote>:<ref>`: Repeatable flag to install Flatpak dependencies (runtimes, SDK extensions) before compiling.
* `--no-install-deps`: Disable automatic injection of `--install-deps-from` flags when running `flatpak-builder`.
* `--no-flathub`: Disable automatic injection of the `flathub` remote as a dependency source.

#### `import`
Ingests prebuilt bundles (`.flatpak`) and rebinds channels:
```bash
aetherpak import --app-id org.example.App --bundle-url https://... --bundle-sha256 <hex>
```
Options:
* `--app-id <app-id>`: target application ID. Supports the Flatpak standard `app-id//branch` ref format (e.g. `org.example.App//beta`) to override its target branch.

#### `push-oci`
Converts repo branch to OCI image layer and pushes:
```bash
aetherpak push-oci --app-id org.example.App --registry ghcr.io --oci-repository my-org/my-app
```
Options:
* `--app-id <app-id>`: target application ID. Supports the Flatpak standard `app-id//branch` ref format (e.g. `org.example.App//beta`) to filter repository refs or override its target branch.
* `--gpg-key <path>`: Local path to GPG private key used to sign image manifests.
* `--no-sign`: Disable GPG signing entirely (bypasses GPG signature step).
* `--allow-unsigned`: Allow pushing unsigned images if signing keys are missing.
* `--dry-run`: Simulate pushing OCI image without writing to remote registry or records.
* `--image-deltas` / `--no-image-deltas`: Enable or disable image delta publishing for this push.
* `--delta-back <n>`: Number of previous releases to use as delta sources (1-10, default from config).

#### `deltas prune`
Removes stale delta and delta-source tags and rebuilds `_deltaindex`:
```bash
aetherpak deltas prune --registry ghcr.io --oci-repository my-org/my-app --dry-run
```
Options:
* `--registry <host>`: Target OCI registry host. Defaults to configuration or the SCM convention.
* `--oci-repository <path>`: Target repository path. Defaults to configuration or the SCM convention.
* `--insecure`: Allow an HTTP registry connection.
* `--dry-run`: Report what would be removed without deleting anything.
* `--output-file <path>`: Write `delta-tags`, `source-tags`, and `index-rebuilt` as dotenv output.

Run it as a serialized maintenance step with registry write credentials. It only removes tags whose manifest is definitively gone or whose source is no longer referenced; transient errors leave them in place.

#### `build-site`
Downloads old static index, merges recent cell records, and regenerates index listings:
```bash
aetherpak build-site --pages-url https://flatpak.my-org.com --site-dir _site --reconcile --index-template templates/custom_index.html
```
Options:
* `--gpg-key <path>`: Local path to GPG private key used to export GPG public keys.
* `--no-sign`: Disable GPG signing and metadata export entirely.
* `--allow-unsigned`: Allow building unsigned index if GPG keys are missing.

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

#### `config`
Gets, sets, or displays configuration parameter settings:
```bash
# Get a top-level configuration value
aetherpak config get remote_name
aetherpak config get branding.logo_url

# Get app-specific configuration in multi-app repositories
aetherpak config get apps.org.example.App.manifest
aetherpak config get --app-id org.example.App manifest
aetherpak config get --app-id org.example.App  # returns full app configuration as YAML

# Set a configuration value
aetherpak config set remote_name custom-remote
aetherpak config set branding.logo_url https://new-logo.png

# Show resolved configuration and active overrides
aetherpak config show
```
Options for `config get`:
* `--app-id <app-id>`: target application ID for multi-app configuration lookups (supports Flatpak `app-id//branch` ref format).

### Porcelain Commands

#### `add`
Creates or modifies an `aetherpak.yaml` by adding one application from a local
manifest, a remote bundle URL (downloaded and fingerprinted), or a git
repository (added as a submodule, initialised recursively). Runs an interactive
wizard on a TTY; otherwise reads flags. A colored diff is shown before changes
are written unless `--confirm`/`-y` is given.
```bash
# Local manifest (app id detected from the manifest)
aetherpak add --manifest org.example.App.yaml

# Bundle URL (downloaded + fingerprinted; SHA-256 recorded)
aetherpak add --bundle-url https://example.com/app.flatpak --app-id org.example.App

# Git repository added as a submodule
aetherpak add --git https://example.com/repo.git

# Skip the diff confirmation
aetherpak add --manifest org.example.App.yaml -y
```
Options:
* `--manifest <path>` / `--bundle-url <url>` / `--git <url>`: the source (exactly one in non-interactive mode).
* `--git-manifest <path>`: manifest path within the git repo (auto-detected if omitted).
* `--submodule-path <path>`: submodule destination (default `manifests/<reponame>`).
* `--app-id`, `--branch` (Flatpak release channel, default `stable`), `--arch` (repeatable, defaults to the host architecture): overrides; `app-id` is derived from the manifest when omitted (only the bundle source requires `--app-id`).
* `--bundle-sha256 <hex>`: expected bundle checksum (verified; computed when omitted).
* Build options (manifest/git sources only): `--install-deps-from-flathub` (default `true`, appends `--install-deps-from=flathub`), `--run-linter`, `--ccache`, and `--builder-arg <arg>` (repeatable, free-form).
* `-y`, `--confirm`: skip the diff confirmation prompt.

The app id is read from the manifest — you are never prompted for it when it can be detected; a manifest that is found but lacks an id is reported as an error. The target config is resolved from `--config`/`AETHERPAK_CONFIG`, else an existing `aetherpak.yaml`/`aetherpak.yml` in the working directory, else a new `aetherpak.yaml` is created.

#### `publish`
Chains compilation/importer and OCI push sequentially in-memory for target application(s):
```bash
# Config-driven publish
aetherpak publish --app-id org.example.App --registry ghcr.io

# One-off publish from local manifest
aetherpak publish --manifest apps/manifest.json --arch x86_64

# One-off publish from Flatpak bundle (URL or local path)
aetherpak publish --bundle https://example.com/app.flatpak
```
Options:
* `--app-id <id>`: target application ID. Supports the Flatpak standard `app-id//branch` ref format (e.g. `org.example.App//beta`) to override its target branch.
* `--manifest <path>`: path to a local Flatpak manifest file (one-off publish, bypasses config).
* `--bundle <url|path>`: Flatpak bundle URL or path to import and publish (one-off publish, bypasses config).
* `--confirm`: skip interactive confirmation prompt when importing bundles.
* `--arch <arch>`: target CPU architecture (defaults to host architecture).
* `--gpg-key <path>`: Local path to GPG private key.
* `--no-sign`: Disable GPG signing entirely.
* `--allow-unsigned`: Allow publishing unsigned images if GPG keys are missing.
* `--linter-exceptions-file <path>`: Local path to linter exceptions file (JSON).
* `--linter-exception <rule>`: Repeatable flag to specify linter exceptions to ignore.
* `--dry-run`: Simulate publishing without writing to remote registry or records.

#### `release`
Coordinates the entire lifecycle: runs matrix planner, compiles/imports changed records concurrently, pushes artifacts, and builds site index layouts:
```bash
aetherpak release --base-sha <sha> --workers 4 --index-template templates/custom_index.html
```
Options:
* `--gpg-key <path>`: Local path to GPG private key.
* `--no-sign`: Disable GPG signing entirely.
* `--allow-unsigned`: Allow releasing unsigned images/index if GPG keys are missing.
* `--linter-exceptions-file <path>`: Local path to linter exceptions file (JSON).
* `--linter-exception <rule>`: Repeatable flag to specify linter exceptions to ignore.
* `--flatpak-remote <name>=<url>`: Repeatable flag to register Flatpak remotes before compiling.
* `--flatpak-dep <remote>:<ref>`: Repeatable flag to install Flatpak dependencies before compiling.
* `--dry-run`: Simulate release process without writing to remote registry, records, or site directories.

#### `status`
Validates that required system dependencies are available, checks configuration files, and decrypts/verifies GPG keys:
```bash
aetherpak status
```
Options:
* `--gpg-key <path>`: Local path to GPG private key block(s) or file(s) to verify signing.
* `--gpg-key-passphrase <passphrase>`: GPG key passphrase to test decryption.
* `--json`: Outputs raw diagnostics status as JSON for script parsing.

#### `preview`
Generates and serves a local landing page template preview using customizable dummy mock data or a live production index:
```bash
# Serve preview locally on port 8080 (default)
aetherpak preview

# Serve preview on port 9000 with a custom template
aetherpak preview --port 9000 --template templates/custom.html

# Generate static preview pages without serving (useful for CI/CD test site generation)
aetherpak preview --no-serve --apps=single --gpg=false
```
Options:
* `--template, -t <path>`: Local path to custom HTML repository index template (overrides configuration template).
* `--default-template`: Force using the default embedded landing page template, ignoring configuration/env settings.
* `--live`: Fetch live production index data instead of generating dummy data.
* `--live-url <url>`: Live Pages URL to download the index from (implicitly enables `--live`).
* `--gpg`: Enable GPG signing simulation for dummy data (defaults to `true`).
* `--apps <single|multiple>`: Simulate single or multiple applications in dummy data (defaults to `multiple`).
* `--site-dir <path>`: Destination directory for preview assets (defaults to `_preview`).
* `--no-serve`: Do not start a local HTTP server to preview the site (defaults to `false`).
* `--port <port>`: Port for local HTTP server (defaults to `8080`).

#### `snippets`
Generates ready-to-use installation and channel-switching documentation snippets for Stable, Nightly, and Flatpakref one-click installs based on your repository configuration:
```bash
# Generate default Markdown snippets
aetherpak snippets

# Generate HTML snippets for a specific application
aetherpak snippets --format=html --app=org.example.App

# Generate structured JSON metadata for custom static site generators
aetherpak snippets --format=json

# Write Markdown snippets directly to a file
aetherpak snippets --format=markdown --output-file=INSTALL.md
```
Options:
* `--format, -f <markdown|html|json>`: Output format (`markdown` [default], `html`, or `json`).
* `--app, -a <app-id>`: Filter snippets by application ID.
* `--channel, -c <channel>`: Filter snippets by channel/branch (e.g. `stable`, `nightly`).
* `--pages-url <url>`: Override base Pages repository URL (defaults to `pages_url` in config).
* `--remote-name <name>`: Override Flatpak remote repository name (defaults to `remote_name` in config).
* `--no-sign`: Generate unverified/unsigned repository command syntax (`--no-gpg-verify`).
* `--output-file, -o <path>`: Write generated snippets to the specified file path (`-` or empty for stdout).

#### `clean`
Clears builder compiler caches, state directories, template preview files, production site outputs, build-site records, and local OSTree repositories.
Requires confirmation unless `--yes`, `-y`, or `--confirm` is specified.
```bash
# Clean all temporary files (prompts for confirmation if interactive)
aetherpak clean

# Clean all temporary files without prompt (useful for CI)
aetherpak clean --yes

# Clean only compiler caches (ccache) and state directories
aetherpak clean --ccache --state --yes
```
Options:
* `--yes, -y`: Skip confirmation prompt.
* `--confirm`: Skip confirmation prompt (deprecated alias for `--yes`).
* `--ccache`: Clean only builder compiler caches (`.ccache`).
* `--state`: Clean only builder state directories (`.state`).
* `--preview`: Clean only template preview directories (`_preview`).
* `--site`: Clean only production site build outputs (`_site`).
* `--records`: Clean only build-site input/output records (`records`).
* `--repo`: Clean only local OSTree repositories (`repo`).
* `--ccache-dir <path>`: Custom ccache directory path (defaults to `.ccache`).
* `--state-dir <path>`: Custom builder state directory path (defaults to `.state`).
* `--preview-dir <path>`: Custom preview site directory path (defaults to `_preview`).
* `--site-dir <path>`: Custom production site directory path (defaults to `_site`).
* `--records-dir <path>`: Custom records directory path (defaults to `records`).
* `--repo-path <path>`: Custom OSTree repository path (defaults to `repo`).

---

## Development

Guidelines on compilation setup, local prerequisites, test harness drivers, and coding styles are documented in [CONTRIBUTING.md](CONTRIBUTING.md).
