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
* **`oci_repository`** (string): The target repository path/name for OCI distribution (replaces deprecated `remote_name` for OCI registry pushes).
* **`remote_name`** (string): The repository name configured in user Flatpak clients (defaults to `<owner>-<repo>`). Historically used for OCI registry pushes; now acts as a fallback for `oci_repository` (deprecated for registry pushes).
* **`output_dir`** (string): Base directory for all output assets (state, records, site, ccache, repo) unless overridden.
* **`no_sign`** (boolean): Set to `true` to disable GPG signing of repositories and OCI images entirely (defaults to `false`).
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
* **`builder_args`** (list[string]): Additional command-line flags to pass directly to `flatpak-builder`.
* **`remotes`** (map[string]string): Map of Flatpak remote repository names to their flatpakrepo URLs (pre-registered before build).
* **`flatpaks`** (list[FlatpakDep]): Flatpak runtimes or SDK extensions/dependencies to pre-install before build. Each entry requires a `remote` (string) and a `ref` (string).

#### `apps`
A list of applications managed in the repository. Each entry supports the following settings:
* **`id`** (string, required): The reverse-DNS Flatpak application identifier (e.g. `org.example.App`).
* **`branch`** (string): The release channel branch (defaults to `stable`).
* **`arches`** (list[string]): Target architectures to compile/import (defaults to `[x86_64]`).
* **`manifest`** (string): Local relative path to the Flatpak manifest file (required for source-based builds).
* **`runtime`** (string): Upstream runtime dependencies list (required for source-based builds).
* **`run-linter`** (boolean): Local toggle to execute linter validation checks.
* **`linter`** (block): Override block for linter strictness, ignore rules, and exceptions. Supports `strict` (boolean), `ignore_rules` (list[string]), `exceptions` (list[string]), and `exceptions_file` (string).
* **`ccache`** / **`ccache_dir`** / **`state_dir`** / **`builder_args`**: Application-specific overrides for compilation parameters.
* **`remotes`** / **`flatpaks`**: Application-specific overrides/merges for Flatpak remotes and dependencies.
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
  remotes:
    flathub: https://dl.flathub.org/repo/flathub.flatpakrepo
  flatpaks:
    - remote: flathub
      ref: org.gnome.Sdk//45

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
    remotes:
      repoA: https://example.com/repoA.flatpakrepo
    flatpaks:
      - remote: repoA
        ref: org.gnome.Sdk.ExtensionA//45

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

#### `build`
Wraps `flatpak-builder` sandbox compilation:
```bash
aetherpak build --app-id org.example.App --manifest apps/manifest.json --arch x86_64 --linter-exception appstream-screenshot-missing --flatpak-remote flathub=https://dl.flathub.org/repo/flathub.flatpakrepo --flatpak-dep flathub:org.gnome.Sdk//45
```
Options:
* `--flatpak-remote <name>=<url>`: Repeatable flag to register Flatpak remotes before compiling.
* `--flatpak-dep <remote>:<ref>`: Repeatable flag to install Flatpak dependencies (runtimes, SDK extensions) before compiling.

#### `import`
Ingests prebuilt bundles (`.flatpak`) and rebinds channels:
```bash
aetherpak import --app-id org.example.App --bundle-url https://... --bundle-sha256 <hex>
```

#### `push-oci`
Converts repo branch to OCI image layer and pushes:
```bash
aetherpak push-oci --app-id org.example.App --registry ghcr.io --oci-repository my-org/my-app
```
Options:
* `--gpg-key <path>`: Local path to GPG private key used to sign image manifests.
* `--no-sign`: Disable GPG signing entirely (bypasses GPG signature step).
* `--allow-unsigned`: Allow pushing unsigned images if signing keys are missing.

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
* `--app-id <id>`: target application ID.
* `--manifest <path>`: path to a local Flatpak manifest file (one-off publish, bypasses config).
* `--bundle <url|path>`: Flatpak bundle URL or path to import and publish (one-off publish, bypasses config).
* `--confirm`: skip interactive confirmation prompt when importing bundles.
* `--arch <arch>`: target CPU architecture (defaults to host architecture).
* `--gpg-key <path>`: Local path to GPG private key.
* `--no-sign`: Disable GPG signing entirely.
* `--allow-unsigned`: Allow publishing unsigned images if GPG keys are missing.
* `--linter-exceptions-file <path>`: Local path to linter exceptions file (JSON).
* `--linter-exception <rule>`: Repeatable flag to specify linter exceptions to ignore.

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

#### `status`
Validates that required system dependencies are available, checks configuration files, and decrypts/verifies GPG keys:
```bash
aetherpak status
```
Options:
* `--gpg-key <path>`: Local path to GPG private key block(s) or file(s) to verify signing.
* `--gpg-key-passphrase <passphrase>`: GPG key passphrase to test decryption.
* `--json`: Outputs raw diagnostics status as JSON for script parsing.

---

## Development

Guidelines on compilation setup, local prerequisites, test harness drivers, and coding styles are documented in [CONTRIBUTING.md](CONTRIBUTING.md).
