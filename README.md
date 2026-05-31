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
aetherpak add --bundle-url https://example.com/app.flatpak --id org.example.App

# Git repository added as a submodule
aetherpak add --git https://example.com/repo.git

# Skip the diff confirmation
aetherpak add --manifest org.example.App.yaml -y
```
Options:
* `--manifest <path>` / `--bundle-url <url>` / `--git <url>`: the source (exactly one in non-interactive mode).
* `--git-manifest <path>`: manifest path within the git repo (auto-detected if omitted).
* `--submodule-path <path>`: submodule destination (default `manifests/<reponame>`).
* `--id`, `--branch` (Flatpak release channel, default `stable`), `--arch` (repeatable, defaults to the host architecture): overrides; `id` is derived from the manifest when omitted (only the bundle source requires `--id`).
* `--bundle-sha256 <hex>`: expected bundle checksum (verified; computed when omitted).
* Build options (manifest/git sources only): `--install-deps-from-flathub` (default `true`, appends `--install-deps-from=flathub`), `--run-linter`, `--ccache`, and `--builder-arg <arg>` (repeatable, free-form).
* `-y`, `--confirm`: skip the diff confirmation prompt.

The app id is read from the manifest — you are never prompted for it when it can be detected; a manifest that is found but lacks an id is reported as an error. The target config is resolved from `--config`/`AETHERPAK_CONFIG`, else an existing `aetherpak.yaml`/`aetherpak.yml` in the working directory, else a new `aetherpak.yaml` is created.

#### `publish`
Chains compilation/importer and OCI push sequentially in-memory for a single target application:
```bash
aetherpak publish --app org.example.App --registry ghcr.io
```
Options:
* `--gpg-key <path>`: Local path to GPG private key.
* `--no-sign`: Disable GPG signing entirely.
* `--allow-unsigned`: Allow publishing unsigned images if GPG keys are missing.

#### `release`
Coordinates the entire lifecycle: runs matrix planner, compiles/imports changed records concurrently, pushes artifacts, and builds site index layouts:
```bash
aetherpak release --base-sha <sha> --workers 4 --index-template templates/custom_index.html
```
Options:
* `--gpg-key <path>`: Local path to GPG private key.
* `--no-sign`: Disable GPG signing entirely.
* `--allow-unsigned`: Allow releasing unsigned images/index if GPG keys are missing.

---

## Development

Guidelines on compilation setup, local prerequisites, test harness drivers, and coding styles are documented in [CONTRIBUTING.md](CONTRIBUTING.md).
