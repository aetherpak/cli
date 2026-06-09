package plan

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aetherpak/aetherpak/pkg/config"
	"gopkg.in/yaml.v3"
)

func manifestApp(id, branch, manifest string, arches ...string) config.App {
	return config.App{ID: id, Branch: branch, Manifest: manifest, Runtime: "gnome-50", Arches: arches}
}

func bundleApp(id, branch string, bundles map[string]config.Bundle) config.App {
	return config.App{ID: id, Branch: branch, Bundles: bundles}
}

func TestComputePlanForceAll(t *testing.T) {
	cfg := &config.Config{Apps: []config.App{
		manifestApp("org.a.One", "stable", "apps/one/m.json", "x86_64"),
		bundleApp("org.b.Two", "beta", map[string]config.Bundle{"x86_64": {URL: "https://e/x", SHA256: "ab"}}),
	}}
	res, err := ComputePlan(cfg, "aetherpak.yaml", "", "all", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Count != 2 || res.CountManifest != 1 || res.CountBundle != 1 {
		t.Fatalf("counts: total=%d manifest=%d bundle=%d", res.Count, res.CountManifest, res.CountBundle)
	}
	if len(res.Apps) != 2 {
		t.Fatalf("apps: %v", res.Apps)
	}
}

func TestComputePlanForceSingle(t *testing.T) {
	cfg := &config.Config{Apps: []config.App{
		manifestApp("org.a.One", "stable", "apps/one/m.json", "x86_64"),
		bundleApp("org.b.Two", "beta", map[string]config.Bundle{"x86_64": {URL: "https://e/x", SHA256: "ab"}}),
	}}
	res, err := ComputePlan(cfg, "aetherpak.yaml", "", "org.b.Two", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Count != 1 || res.Apps[0] != "org.b.Two" {
		t.Fatalf("expected only org.b.Two, got %v", res.Apps)
	}
	if res.CountManifest != 0 || res.CountBundle != 1 {
		t.Fatalf("source split wrong: manifest=%d bundle=%d", res.CountManifest, res.CountBundle)
	}
}

func TestComputePlanForceUnknownErrors(t *testing.T) {
	cfg := &config.Config{Apps: []config.App{manifestApp("org.a.One", "stable", "apps/one/m.json", "x86_64")}}
	if _, err := ComputePlan(cfg, "aetherpak.yaml", "", "org.does.NotExist", ""); err == nil {
		t.Fatal("expected error for unknown forced app")
	}
}

func TestComputePlanManifestExpansion(t *testing.T) {
	cfg := &config.Config{Apps: []config.App{
		{ID: "org.a.One", Branch: "stable", Manifest: "apps/one/m.json", Runtime: "gnome-50", Arches: []string{"x86_64", "aarch64"}, RunLinter: true},
	}}
	res, err := ComputePlan(cfg, "aetherpak.yaml", "", "all", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(res.MatrixManifest) != 2 || len(res.MatrixBundle) != 0 {
		t.Fatalf("expected 2 manifest rows, 0 bundle: %+v", res)
	}
	byArch := map[string]MatrixRow{}
	for _, r := range res.MatrixManifest {
		byArch[r.Arch] = r
	}
	x := byArch["x86_64"]
	if x.Source != "manifest" || x.Runner != "ubuntu-latest" || x.Manifest != "apps/one/m.json" || x.Runtime != "gnome-50" || x.Branch != "stable" || !x.RunLinter {
		t.Fatalf("x86_64 row wrong: %+v", x)
	}
	if byArch["aarch64"].Runner != "ubuntu-24.04-arm" {
		t.Fatalf("aarch64 runner wrong: %q", byArch["aarch64"].Runner)
	}
}

func TestComputePlanManifestArchDefaultAndBranchDefault(t *testing.T) {
	cfg := &config.Config{Apps: []config.App{
		{ID: "org.a.One", Manifest: "m.json", Runtime: "gnome-50"}, // no arches, no branch
	}}
	res, err := ComputePlan(cfg, "aetherpak.yaml", "", "all", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(res.MatrixManifest) != 1 {
		t.Fatalf("expected 1 row: %+v", res.MatrixManifest)
	}
	row := res.MatrixManifest[0]
	if row.Arch != "x86_64" || row.Branch != "stable" {
		t.Fatalf("defaults not applied: arch=%q branch=%q", row.Arch, row.Branch)
	}
}

func TestComputePlanBundleExpansion(t *testing.T) {
	cfg := &config.Config{Apps: []config.App{
		bundleApp("org.b.Two", "", map[string]config.Bundle{
			"x86_64":  {URL: "https://e/x", SHA256: "aa"},
			"aarch64": {URL: "https://e/a", SHA256: "bb"},
		}),
	}}
	res, err := ComputePlan(cfg, "aetherpak.yaml", "", "all", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(res.MatrixBundle) != 2 || len(res.MatrixManifest) != 0 {
		t.Fatalf("expected 2 bundle rows: %+v", res)
	}
	got := map[string]MatrixRow{}
	for _, r := range res.MatrixBundle {
		got[r.Arch] = r
	}
	if got["x86_64"].BundleURL != "https://e/x" || got["x86_64"].BundleSHA256 != "aa" || got["x86_64"].Branch != "stable" {
		t.Fatalf("x86_64 bundle row wrong: %+v", got["x86_64"])
	}
	if got["aarch64"].Runner != "ubuntu-24.04-arm" || got["aarch64"].Source != "bundle" {
		t.Fatalf("aarch64 bundle row wrong: %+v", got["aarch64"])
	}
}

func TestAppConfigsEqual(t *testing.T) {
	a := manifestApp("org.a.One", "stable", "m.json", "x86_64")
	if !appConfigsEqual(a, a) {
		t.Fatal("identical apps should be equal")
	}
	b := a
	b.Branch = "beta"
	if appConfigsEqual(a, b) {
		t.Fatal("differing branch should not be equal")
	}
}

// --- change detection (git-backed) ---

func runGit(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFiles(t *testing.T, files map[string]string) {
	t.Helper()
	for p, c := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// setupRepo creates a temp git repo, commits base, then commits head, and
// chdirs the test into it. Returns the base commit SHA.
func setupRepo(t *testing.T, base, head map[string]string) string {
	t.Helper()
	t.Chdir(t.TempDir())
	runGit(t, "init", "-q")
	writeFiles(t, base)
	runGit(t, "add", "-A")
	runGit(t, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "base")
	baseSHA := runGit(t, "rev-parse", "HEAD")
	writeFiles(t, head)
	runGit(t, "add", "-A")
	runGit(t, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "head")
	return baseSHA
}

func parseCfg(t *testing.T, y string) *config.Config {
	t.Helper()
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(y), &cfg); err != nil {
		t.Fatalf("parse cfg: %v", err)
	}
	return &cfg
}

const twoAppYAML = `apps:
  - id: org.a.One
    manifest: apps/one/m.json
    runtime: gnome-50
    arches: [x86_64]
  - id: org.b.Two
    manifest: apps/two/m.json
    runtime: gnome-50
    arches: [x86_64]
`

func TestChangeDetectionManifestDirTouched(t *testing.T) {
	base := map[string]string{
		"aetherpak.yaml":    twoAppYAML,
		"apps/one/m.json":   "{}",
		"apps/two/m.json":   "{}",
		"apps/one/data.txt": "v1",
	}
	head := map[string]string{"apps/one/data.txt": "v2"} // only One's dir changes
	baseSHA := setupRepo(t, base, head)

	res, err := ComputePlan(parseCfg(t, twoAppYAML), "aetherpak.yaml", baseSHA, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Count != 1 || res.Apps[0] != "org.a.One" {
		t.Fatalf("only org.a.One should be selected, got %v", res.Apps)
	}
}

func TestChangeDetectionUnrelatedChangeSelectsNothing(t *testing.T) {
	base := map[string]string{
		"aetherpak.yaml":  twoAppYAML,
		"apps/one/m.json": "{}",
		"apps/two/m.json": "{}",
		"README.md":       "v1",
	}
	head := map[string]string{"README.md": "v2"} // outside any manifest dir, config unchanged
	baseSHA := setupRepo(t, base, head)

	res, err := ComputePlan(parseCfg(t, twoAppYAML), "aetherpak.yaml", baseSHA, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Count != 0 {
		t.Fatalf("no app should be selected, got %v", res.Apps)
	}
}

func TestChangeDetectionConfigChangeSelectsApp(t *testing.T) {
	headYAML := strings.Replace(twoAppYAML, "id: org.b.Two\n    manifest: apps/two/m.json\n    runtime: gnome-50\n    arches: [x86_64]",
		"id: org.b.Two\n    manifest: apps/two/m.json\n    runtime: gnome-50\n    arches: [x86_64, aarch64]", 1)
	base := map[string]string{"aetherpak.yaml": twoAppYAML, "apps/one/m.json": "{}", "apps/two/m.json": "{}"}
	head := map[string]string{"aetherpak.yaml": headYAML} // Two gains an arch; no manifest dir touched
	baseSHA := setupRepo(t, base, head)

	res, err := ComputePlan(parseCfg(t, headYAML), "aetherpak.yaml", baseSHA, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Two's config changed -> selected; One unchanged -> not selected.
	if res.Count != 1 || res.Apps[0] != "org.b.Two" {
		t.Fatalf("only org.b.Two should be selected, got %v", res.Apps)
	}
}

func TestChangeDetectionNewAppSelected(t *testing.T) {
	oneAppYAML := "apps:\n  - id: org.a.One\n    manifest: apps/one/m.json\n    runtime: gnome-50\n    arches: [x86_64]\n"
	base := map[string]string{"aetherpak.yaml": oneAppYAML, "apps/one/m.json": "{}"}
	head := map[string]string{"aetherpak.yaml": twoAppYAML, "apps/two/m.json": "{}"} // adds org.b.Two
	baseSHA := setupRepo(t, base, head)

	res, err := ComputePlan(parseCfg(t, twoAppYAML), "aetherpak.yaml", baseSHA, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// One is unchanged config + untouched dir -> not selected; Two is new -> selected.
	if res.Count != 1 || res.Apps[0] != "org.b.Two" {
		t.Fatalf("only new app org.b.Two should be selected, got %v", res.Apps)
	}
}

func TestChangeDetectionWorkflowChangeSelectsAll(t *testing.T) {
	base := map[string]string{
		"aetherpak.yaml":          twoAppYAML,
		"apps/one/m.json":         "{}",
		"apps/two/m.json":         "{}",
		".github/workflows/p.yml": "v1",
	}
	head := map[string]string{".github/workflows/p.yml": "v2"}
	baseSHA := setupRepo(t, base, head)

	res, err := ComputePlan(parseCfg(t, twoAppYAML), "aetherpak.yaml", baseSHA, "", ".github/workflows/p.yml")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Count != 2 {
		t.Fatalf("workflow change should select all, got %v", res.Apps)
	}
}

func TestChangeDetectionNoBaseSelectsAll(t *testing.T) {
	cfg := parseCfg(t, twoAppYAML)
	res, err := ComputePlan(cfg, "aetherpak.yaml", "", "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Count != 2 {
		t.Fatalf("empty base SHA should select all, got %v", res.Apps)
	}
}

func TestChangeDetectionUnreachableBaseSelectsAll(t *testing.T) {
	base := map[string]string{"aetherpak.yaml": twoAppYAML, "apps/one/m.json": "{}", "apps/two/m.json": "{}"}
	head := map[string]string{"README.md": "x"}
	setupRepo(t, base, head)

	res, err := ComputePlan(parseCfg(t, twoAppYAML), "aetherpak.yaml", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Count != 2 {
		t.Fatalf("unreachable base SHA should select all, got %v", res.Apps)
	}
}

func TestChangeDetectionNormalizationSymmetry(t *testing.T) {
	implicitYAML := `apps:
  - id: org.a.One
    manifest: apps/one/m.json
    runtime: gnome-50
`
	base := map[string]string{
		"aetherpak.yaml":  implicitYAML,
		"apps/one/m.json": "{}",
		"README.md":       "v1",
	}
	head := map[string]string{
		"README.md": "v2",
	}
	baseSHA := setupRepo(t, base, head)

	res, err := ComputePlan(parseCfg(t, implicitYAML), "aetherpak.yaml", baseSHA, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Count != 0 {
		t.Fatalf("expected 0 apps selected for unrelated change with implicit config defaults, got %v", res.Apps)
	}
}

func TestComputePlanManifestBranchFromManifest(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	manifestContent := `{"id":"org.a.One","runtime":"org.freedesktop.Platform","runtime-version":"24.08","branch":"25.08"}`
	if err := os.WriteFile("m.json", []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Apps: []config.App{
		{ID: "org.a.One", Manifest: "m.json", Runtime: "gnome-50"},
	}}
	res, err := ComputePlan(cfg, "aetherpak.yaml", "", "all", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(res.MatrixManifest) != 1 {
		t.Fatalf("expected 1 row: %+v", res.MatrixManifest)
	}
	row := res.MatrixManifest[0]
	if row.Branch != "25.08" {
		t.Errorf("Branch = %q, want 25.08 resolved from manifest file", row.Branch)
	}
}
