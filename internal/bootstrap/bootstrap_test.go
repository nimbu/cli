package bootstrap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifestRejectsUnknownRecipeReference(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "bootstrap", "manifest.yml"), strings.TrimSpace(`
base_paths:
  - nimbu.yml
repeatables:
  - id: header
    label: Header
    paths:
      - snippets/repeatables/header.liquid
    recipes:
      - missing
`))

	_, err := LoadManifest(root)
	if err == nil || !strings.Contains(err.Error(), "unknown recipe") {
		t.Fatalf("expected unknown recipe error, got %v", err)
	}
}

func TestLoadManifestRejectsBothBasePathsAndExclude(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "bootstrap", "manifest.yml"), strings.TrimSpace(`
base_paths:
  - nimbu.yml
exclude:
  - bootstrap/
repeatables: []
recipes: []
bundles: []
`))

	_, err := LoadManifest(root)
	if err == nil || !strings.Contains(err.Error(), "cannot have both") {
		t.Fatalf("expected 'cannot have both' error, got %v", err)
	}
}

func TestLoadManifestAcceptsExcludeOnly(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "bootstrap", "manifest.yml"), strings.TrimSpace(`
name: Starterskit
exclude:
  - "*.md"
  - .github/
repeatables: []
recipes: []
bundles: []
`))

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(manifest.Exclude) != 2 {
		t.Fatalf("expected 2 exclude patterns, got %d", len(manifest.Exclude))
	}
}

func TestLoadManifestAcceptsNeitherBasePathsNorExclude(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "bootstrap", "manifest.yml"), strings.TrimSpace(`
name: Minimal
repeatables: []
recipes: []
bundles: []
`))

	_, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("expected no error for empty exclude mode, got %v", err)
	}
}

func TestLoadManifestReadsTemplateName(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "bootstrap", "manifest.yml"), strings.TrimSpace(`
name: Starterskit
base_paths:
  - nimbu.yml
repeatables: []
recipes: []
bundles: []
`))

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if manifest.Name != "Starterskit" {
		t.Fatalf("expected manifest name to be loaded, got %q", manifest.Name)
	}
}

func TestBootstrapProjectCopiesSelectedFilesAndStripsUnselectedRepeatables(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()
	writeStarterFixture(t, sourceDir)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	result, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: filepath.Join(destDir, "theme-demo"),
		Site:           "demo-site",
		Theme:          "storefront",
		RepeatableIDs:  []string{"text"},
	})
	if err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	if result.Path == "" {
		t.Fatal("expected bootstrap result path")
	}

	pageData, err := os.ReadFile(filepath.Join(result.Path, "templates", "page.liquid"))
	if err != nil {
		t.Fatalf("read page template: %v", err)
	}
	page := string(pageData)
	if strings.Contains(page, `{% repeatable "header"`) {
		t.Fatalf("expected header repeatable to be stripped, got:\n%s", page)
	}
	if !strings.Contains(page, `{% repeatable "text"`) {
		t.Fatalf("expected text repeatable to remain, got:\n%s", page)
	}

	if _, err := os.Stat(filepath.Join(result.Path, "snippets", "repeatables", "header.liquid")); !os.IsNotExist(err) {
		t.Fatalf("expected header snippet to be omitted, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "snippets", "repeatables", "text.liquid")); err != nil {
		t.Fatalf("expected text snippet to exist: %v", err)
	}

	projectData, err := os.ReadFile(filepath.Join(result.Path, "nimbu.yml"))
	if err != nil {
		t.Fatalf("read nimbu.yml: %v", err)
	}
	project := string(projectData)
	if !strings.Contains(project, "site: demo-site") || !strings.Contains(project, "theme: storefront") {
		t.Fatalf("expected project config rewrite, got:\n%s", project)
	}
}

func TestBootstrapProjectGeneratesRecipeIndexesFromSelectedRecipes(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()
	writeStarterFixture(t, sourceDir)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	result, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: filepath.Join(destDir, "theme-demo"),
		Site:           "demo-site",
		Theme:          "storefront",
		BundleIDs:      []string{"customer-auth"},
		RepeatableIDs:  []string{"text"},
	})
	if err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	runtimeIndex := readTestFile(t, filepath.Join(result.Path, "src", "recipes", "index.ts"))
	if !strings.Contains(runtimeIndex, "import AlpineFilePond") {
		t.Fatalf("expected runtime recipe import, got:\n%s", runtimeIndex)
	}
	if !strings.Contains(runtimeIndex, "alpine.data('file', AlpineFilePond)") {
		t.Fatalf("expected runtime registration, got:\n%s", runtimeIndex)
	}

	cssIndex := readTestFile(t, filepath.Join(result.Path, "src", "recipes", "index.css"))
	if !strings.Contains(cssIndex, "@import './filepond/index.css';") {
		t.Fatalf("expected css recipe import, got:\n%s", cssIndex)
	}

	buildIndex := readTestFile(t, filepath.Join(result.Path, "build", "recipes", "index.ts"))
	if !strings.Contains(buildIndex, "import filePondRecipe from './filepond';") {
		t.Fatalf("expected build recipe import, got:\n%s", buildIndex)
	}
	if !strings.Contains(buildIndex, "export default [filePondRecipe];") {
		t.Fatalf("expected build recipe export, got:\n%s", buildIndex)
	}
}

func TestBootstrapProjectSkipsMissingGeneratedOutputs(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()
	writeStarterFixture(t, sourceDir)
	writeTestFile(t, filepath.Join(sourceDir, "bootstrap", "manifest.yml"), strings.TrimSpace(`
base_paths:
  - nimbu.yml
  - package.json
  - templates/page.liquid
  - javascripts
  - stylesheets
  - scripts
  - snippets/webpack_app.liquid
repeatables:
  - id: text
    label: Text
    paths:
      - snippets/repeatables/text.liquid
    transforms:
      - type: remove_repeatable
        path: templates/page.liquid
        name: text
`))

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	result, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: filepath.Join(destDir, "theme-demo"),
		Site:           "demo-site",
		Theme:          "storefront",
		RepeatableIDs:  []string{"text"},
	})
	if err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	if _, err := os.Stat(filepath.Join(result.Path, "nimbu.yml")); err != nil {
		t.Fatalf("expected base project to be copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "javascripts")); !os.IsNotExist(err) {
		t.Fatalf("expected missing generated output to stay absent, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "scripts")); !os.IsNotExist(err) {
		t.Fatalf("expected missing legacy scripts dir to stay absent, stat err=%v", err)
	}
}

func TestBootstrapProjectCreatesGeneratedDirsWithGitkeep(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()
	writeStarterFixture(t, sourceDir)
	writeTestFile(t, filepath.Join(sourceDir, "bootstrap", "manifest.yml"), strings.TrimSpace(`
base_paths:
  - nimbu.yml
  - package.json
  - templates/page.liquid
generated_dirs:
  - javascripts
  - stylesheets
repeatables:
  - id: text
    label: Text
    paths:
      - snippets/repeatables/text.liquid
    transforms:
      - type: remove_repeatable
        path: templates/page.liquid
        name: text
`))

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	result, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: filepath.Join(destDir, "theme-demo"),
		Site:           "demo-site",
		Theme:          "storefront",
		RepeatableIDs:  []string{"text"},
	})
	if err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	for _, dir := range []string{"javascripts", "stylesheets"} {
		if info, err := os.Stat(filepath.Join(result.Path, dir)); err != nil || !info.IsDir() {
			t.Fatalf("expected generated dir %s, stat err=%v", dir, err)
		}
		if _, err := os.Stat(filepath.Join(result.Path, dir, ".gitkeep")); err != nil {
			t.Fatalf("expected .gitkeep in %s: %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(result.Path, "snippets", "webpack_app.liquid")); !os.IsNotExist(err) {
		t.Fatalf("expected generated file to remain absent until build, stat err=%v", err)
	}
}

func TestPlanPathsReturnsConcreteFiles(t *testing.T) {
	sourceDir := t.TempDir()
	writeStarterFixture(t, sourceDir)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	plan, err := PlanPaths(BootstrapOptions{
		Manifest:      manifest,
		SourceDir:     sourceDir,
		RepeatableIDs: []string{"text"},
	})
	if err != nil {
		t.Fatalf("plan paths: %v", err)
	}

	got := map[string]struct{}{}
	for _, p := range plan {
		got[p] = struct{}{}
	}
	for _, want := range []string{"nimbu.yml", "package.json", "templates/page.liquid", "snippets/repeatables/text.liquid"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("expected plan to include %q, got %v", want, plan)
		}
	}
	if _, ok := got["snippets/repeatables/header.liquid"]; ok {
		t.Fatalf("expected unselected repeatable to be absent from plan, got %v", plan)
	}
}

func TestBootstrapProjectRefusesExistingWithoutAllowExisting(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()
	writeStarterFixture(t, sourceDir)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := filepath.Join(destDir, "theme-demo")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	_, err = BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: target,
		Site:           "demo-site",
		Theme:          "storefront",
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected 'already exists' error, got %v", err)
	}
}

func TestBootstrapProjectInPlaceLeavesGitAloneAndHonorsSkipPaths(t *testing.T) {
	sourceDir := t.TempDir()
	writeStarterFixture(t, sourceDir)

	target := t.TempDir()
	// Pre-existing git repo (no commits) and a user-authored nimbu.yml.
	cmd := exec.Command("git", "-C", target, "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	writeTestFile(t, filepath.Join(target, "nimbu.yml"), "site: keep-me\ntheme: keep-me\n")

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	result, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: target,
		Site:           "demo-site",
		Theme:          "storefront",
		RepeatableIDs:  []string{"text"},
		AllowExisting:  true,
		SkipPaths:      map[string]struct{}{"nimbu.yml": {}},
	})
	if err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}
	if result.Path != filepath.Clean(target) {
		t.Fatalf("expected result path %q, got %q", filepath.Clean(target), result.Path)
	}

	// Declined file preserved (not overwritten, not rewritten).
	project := readTestFile(t, filepath.Join(target, "nimbu.yml"))
	if !strings.Contains(project, "site: keep-me") || strings.Contains(project, "demo-site") {
		t.Fatalf("expected user nimbu.yml to be preserved, got:\n%s", project)
	}

	// Non-conflicting files written.
	if _, err := os.Stat(filepath.Join(target, "package.json")); err != nil {
		t.Fatalf("expected package.json to be copied: %v", err)
	}

	// Git left alone: repo still present, no commit created.
	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Fatalf("expected existing .git to be preserved: %v", err)
	}
	count := strings.TrimSpace(gitOutput(t, target, "rev-list", "--all", "--count"))
	if count != "0" {
		t.Fatalf("expected no commits created in existing repo, got %q", count)
	}
}

func TestBootstrapProjectInPlaceOverwritesNonSkippedConflict(t *testing.T) {
	sourceDir := t.TempDir()
	writeStarterFixture(t, sourceDir)

	target := t.TempDir()
	writeTestFile(t, filepath.Join(target, "package.json"), "OLD\n")

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if _, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: target,
		Site:           "demo-site",
		Theme:          "storefront",
		RepeatableIDs:  []string{"text"},
		AllowExisting:  true,
	}); err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	pkg := readTestFile(t, filepath.Join(target, "package.json"))
	if strings.TrimSpace(pkg) != "{}" {
		t.Fatalf("expected package.json to be overwritten with source content, got %q", pkg)
	}
	// nimbu.yml not skipped → rewritten with selected site/theme.
	project := readTestFile(t, filepath.Join(target, "nimbu.yml"))
	if !strings.Contains(project, "site: demo-site") || !strings.Contains(project, "theme: storefront") {
		t.Fatalf("expected nimbu.yml rewrite, got:\n%s", project)
	}
}

func TestPlanPathsIncludesGeneratedRecipeIndexes(t *testing.T) {
	sourceDir := t.TempDir()
	writeStarterFixture(t, sourceDir)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	plan, err := PlanPaths(BootstrapOptions{Manifest: manifest, SourceDir: sourceDir})
	if err != nil {
		t.Fatalf("plan paths: %v", err)
	}
	got := map[string]struct{}{}
	for _, p := range plan {
		got[p] = struct{}{}
	}
	// Generated index files are written by BootstrapProject but not copied, so
	// PlanPaths must still report them for in-place conflict detection.
	for _, want := range []string{"src/recipes/index.ts", "src/recipes/index.css", "build/recipes/index.ts"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("expected plan to include generated %q, got %v", want, plan)
		}
	}
}

func TestBootstrapProjectInPlaceDoesNotClobberDeclinedGeneratedIndex(t *testing.T) {
	sourceDir := t.TempDir()
	writeStarterFixture(t, sourceDir)

	target := t.TempDir()
	const userIndex = "// MY HAND-WRITTEN RECIPE INDEX\n"
	writeTestFile(t, filepath.Join(target, "src", "recipes", "index.ts"), userIndex)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if _, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: target,
		Site:           "demo-site",
		Theme:          "storefront",
		BundleIDs:      []string{"customer-auth"}, // selects the filepond recipe
		RepeatableIDs:  []string{"text"},
		AllowExisting:  true,
		SkipPaths:      map[string]struct{}{"src/recipes/index.ts": {}},
	}); err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	// Declined generated index preserved verbatim.
	if got := readTestFile(t, filepath.Join(target, "src", "recipes", "index.ts")); got != userIndex {
		t.Fatalf("expected declined recipe index preserved, got:\n%s", got)
	}
	// A non-declined generated index is still written.
	if _, err := os.Stat(filepath.Join(target, "build", "recipes", "index.ts")); err != nil {
		t.Fatalf("expected non-declined generated index written: %v", err)
	}
}

func TestBootstrapProjectInPlaceSkipsTransformsOnDeclinedFiles(t *testing.T) {
	sourceDir := t.TempDir()
	writeStarterFixture(t, sourceDir)

	target := t.TempDir()
	// User's own page.liquid lacking the header block. If the "header"
	// remove_repeatable transform ran against it, removeRepeatableBlock would
	// fail with "manifest drift" — so a clean run proves the transform was skipped.
	const userPage = "<main>my own layout</main>\n"
	writeTestFile(t, filepath.Join(target, "templates", "page.liquid"), userPage)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if _, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: target,
		Site:           "demo-site",
		Theme:          "storefront",
		RepeatableIDs:  []string{"text"}, // leaves "header" unselected → header transform would run
		AllowExisting:  true,
		SkipPaths:      map[string]struct{}{"templates/page.liquid": {}},
	}); err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	if got := readTestFile(t, filepath.Join(target, "templates", "page.liquid")); got != userPage {
		t.Fatalf("expected declined page.liquid preserved untouched, got:\n%s", got)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func writeStarterFixture(t *testing.T, root string) {
	t.Helper()

	writeTestFile(t, filepath.Join(root, "bootstrap", "manifest.yml"), strings.TrimSpace(`
base_paths:
  - nimbu.yml
  - package.json
  - src/recipes/filepond/alpine.ts
  - src/recipes/filepond/index.css
  - build/recipes/filepond.ts
  - templates/page.liquid
bundles:
  - id: customer-auth
    label: Customer auth
    paths:
      - templates/customers/login.liquid
    recipes:
      - filepond
recipes:
  - id: filepond
    runtime_identifier: AlpineFilePond
    runtime_import: ./filepond/alpine
    runtime_register_as: file
    css_import: ./filepond/index.css
    build_identifier: filePondRecipe
    build_import: ./filepond
repeatables:
  - id: header
    label: Header
    paths:
      - snippets/repeatables/header.liquid
    transforms:
      - type: remove_repeatable
        path: templates/page.liquid
        name: header
  - id: text
    label: Text
    paths:
      - snippets/repeatables/text.liquid
    transforms:
      - type: remove_repeatable
        path: templates/page.liquid
        name: text
`))
	writeTestFile(t, filepath.Join(root, "nimbu.yml"), "site: old-site\ntheme: old-theme\n")
	writeTestFile(t, filepath.Join(root, "package.json"), "{}\n")
	writeTestFile(t, filepath.Join(root, "templates", "page.liquid"), `<main>
  {% repeatable "header", label: "Header" %}
    {% include "repeatables/header" %}
  {% endrepeatable %}

  {% repeatable "text", label: "Text" %}
    {% include "repeatables/text" %}
  {% endrepeatable %}
</main>
`)
	writeTestFile(t, filepath.Join(root, "templates", "customers", "login.liquid"), "login\n")
	writeTestFile(t, filepath.Join(root, "snippets", "repeatables", "header.liquid"), "header\n")
	writeTestFile(t, filepath.Join(root, "snippets", "repeatables", "text.liquid"), "text\n")
	writeTestFile(t, filepath.Join(root, "src", "recipes", "filepond", "alpine.ts"), "export default function AlpineFilePond() {}\n")
	writeTestFile(t, filepath.Join(root, "src", "recipes", "filepond", "index.css"), ".filepond {}\n")
	writeTestFile(t, filepath.Join(root, "build", "recipes", "filepond.ts"), "export default { name: 'filepond' };\n")
}

func writeTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// --- Exclude-mode integration tests ---

func TestBootstrapProjectExcludeMode(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()
	writeExcludeStarterFixture(t, sourceDir)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	result, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: filepath.Join(destDir, "theme-demo"),
		Site:           "demo-site",
		Theme:          "storefront",
		RepeatableIDs:  []string{"text"},
	})
	if err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	// Base files should be present
	for _, f := range []string{"nimbu.yml", "package.json", filepath.Join("src", "index.ts")} {
		if _, err := os.Stat(filepath.Join(result.Path, f)); err != nil {
			t.Errorf("expected %s to exist: %v", f, err)
		}
	}

	// Excluded files should be absent
	for _, f := range []string{"README.md", filepath.Join(".github", "ci.yml"), "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(result.Path, f)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be excluded, stat err=%v", f, err)
		}
	}

	// bootstrap/ should always be excluded
	if _, err := os.Stat(filepath.Join(result.Path, "bootstrap", "manifest.yml")); !os.IsNotExist(err) {
		t.Errorf("expected bootstrap/ to be excluded, stat err=%v", err)
	}

	// Project config should be rewritten
	projectData, err := os.ReadFile(filepath.Join(result.Path, "nimbu.yml"))
	if err != nil {
		t.Fatalf("read nimbu.yml: %v", err)
	}
	project := string(projectData)
	if !strings.Contains(project, "site: demo-site") || !strings.Contains(project, "theme: storefront") {
		t.Fatalf("expected project config rewrite, got:\n%s", project)
	}
}

func TestBootstrapProjectExcludeWithBundlesAndRepeatables(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()
	writeExcludeStarterFixture(t, sourceDir)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	result, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: filepath.Join(destDir, "theme-demo"),
		Site:           "demo-site",
		Theme:          "storefront",
		BundleIDs:      []string{"customer-auth"},
		RepeatableIDs:  []string{"text"},
	})
	if err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	// Selected bundle path should be present
	if _, err := os.Stat(filepath.Join(result.Path, "templates", "customers", "login.liquid")); err != nil {
		t.Fatalf("expected selected bundle path to exist: %v", err)
	}

	// Selected repeatable path should be present
	if _, err := os.Stat(filepath.Join(result.Path, "snippets", "repeatables", "text.liquid")); err != nil {
		t.Fatalf("expected selected repeatable path to exist: %v", err)
	}

	// Unselected repeatable path should be absent (conditional, not selected)
	if _, err := os.Stat(filepath.Join(result.Path, "snippets", "repeatables", "header.liquid")); !os.IsNotExist(err) {
		t.Fatalf("expected unselected repeatable path to be absent, stat err=%v", err)
	}

	// Unselected repeatable should be stripped from template
	pageData, err := os.ReadFile(filepath.Join(result.Path, "templates", "page.liquid"))
	if err != nil {
		t.Fatalf("read page template: %v", err)
	}
	page := string(pageData)
	if strings.Contains(page, `{% repeatable "header"`) {
		t.Fatalf("expected header repeatable to be stripped, got:\n%s", page)
	}
	if !strings.Contains(page, `{% repeatable "text"`) {
		t.Fatalf("expected text repeatable to remain, got:\n%s", page)
	}

	// Recipe indexes should be generated for selected recipes
	runtimeIndex := readTestFile(t, filepath.Join(result.Path, "src", "recipes", "index.ts"))
	if !strings.Contains(runtimeIndex, "import AlpineFilePond") {
		t.Fatalf("expected runtime recipe import, got:\n%s", runtimeIndex)
	}
}

func TestBootstrapProjectExcludeBootstrapAlwaysExcluded(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()

	// Fixture with empty exclude list — bootstrap/ should still be excluded
	writeTestFile(t, filepath.Join(sourceDir, "bootstrap", "manifest.yml"), strings.TrimSpace(`
exclude: []
generated_dirs:
  - javascripts
repeatables: []
recipes: []
bundles: []
`))
	writeTestFile(t, filepath.Join(sourceDir, "nimbu.yml"), "site: old\ntheme: old\n")
	writeTestFile(t, filepath.Join(sourceDir, "src", "app.ts"), "console.log('hi')\n")
	initGitFixture(t, sourceDir)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	result, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: filepath.Join(destDir, "theme-demo"),
		Site:           "demo-site",
		Theme:          "storefront",
	})
	if err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	if _, err := os.Stat(filepath.Join(result.Path, "bootstrap")); !os.IsNotExist(err) {
		t.Fatalf("expected bootstrap/ to be excluded even with empty exclude list, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "src", "app.ts")); err != nil {
		t.Fatalf("expected src/app.ts to exist: %v", err)
	}
}

func TestBootstrapProjectExcludeGeneratedDirs(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()
	writeExcludeStarterFixture(t, sourceDir)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	result, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: filepath.Join(destDir, "theme-demo"),
		Site:           "demo-site",
		Theme:          "storefront",
		RepeatableIDs:  []string{"text"},
	})
	if err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	for _, dir := range []string{"javascripts", "stylesheets"} {
		if info, err := os.Stat(filepath.Join(result.Path, dir)); err != nil || !info.IsDir() {
			t.Fatalf("expected generated dir %s, stat err=%v", dir, err)
		}
		if _, err := os.Stat(filepath.Join(result.Path, dir, ".gitkeep")); err != nil {
			t.Fatalf("expected .gitkeep in %s: %v", dir, err)
		}
	}
}

func TestBootstrapProjectExcludeSelectedPathsOverrideExclude(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()

	// Bundle declares a .md file; exclude list has *.md.
	// Selected bundle path should still be included.
	writeTestFile(t, filepath.Join(sourceDir, "bootstrap", "manifest.yml"), strings.TrimSpace(`
exclude:
  - "*.md"
bundles:
  - id: docs
    label: Docs
    paths:
      - docs/GUIDE.md
repeatables: []
recipes: []
`))
	writeTestFile(t, filepath.Join(sourceDir, "nimbu.yml"), "site: old\ntheme: old\n")
	writeTestFile(t, filepath.Join(sourceDir, "docs", "GUIDE.md"), "# Guide\n")
	writeTestFile(t, filepath.Join(sourceDir, "README.md"), "# Readme\n")
	initGitFixture(t, sourceDir)

	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	result, err := BootstrapProject(BootstrapOptions{
		Manifest:       manifest,
		SourceDir:      sourceDir,
		DestinationDir: filepath.Join(destDir, "theme-demo"),
		Site:           "demo-site",
		Theme:          "storefront",
		BundleIDs:      []string{"docs"},
	})
	if err != nil {
		t.Fatalf("bootstrap project: %v", err)
	}

	// Selected bundle's .md file should be present despite *.md exclude
	if _, err := os.Stat(filepath.Join(result.Path, "docs", "GUIDE.md")); err != nil {
		t.Fatalf("expected selected bundle path to override exclude: %v", err)
	}
	// Non-selected .md files should still be excluded
	if _, err := os.Stat(filepath.Join(result.Path, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("expected non-selected README.md to be excluded, stat err=%v", err)
	}
}

func TestLoadManifestAcceptsExplicitEmptyBasePaths(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "bootstrap", "manifest.yml"), strings.TrimSpace(`
base_paths: []
repeatables: []
recipes: []
bundles: []
`))

	// Explicit empty base_paths enters exclude mode (same as absent)
	_, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("expected no error for explicit empty base_paths, got %v", err)
	}
}

func writeExcludeStarterFixture(t *testing.T, root string) {
	t.Helper()

	writeTestFile(t, filepath.Join(root, "bootstrap", "manifest.yml"), strings.TrimSpace(`
exclude:
  - "*.md"
  - .github/
  - AGENTS.md
generated_dirs:
  - javascripts
  - stylesheets
bundles:
  - id: customer-auth
    label: Customer auth
    paths:
      - templates/customers/login.liquid
    recipes:
      - filepond
recipes:
  - id: filepond
    runtime_identifier: AlpineFilePond
    runtime_import: ./filepond/alpine
    runtime_register_as: file
    css_import: ./filepond/index.css
    build_identifier: filePondRecipe
    build_import: ./filepond
repeatables:
  - id: header
    label: Header
    paths:
      - snippets/repeatables/header.liquid
    transforms:
      - type: remove_repeatable
        path: templates/page.liquid
        name: header
  - id: text
    label: Text
    paths:
      - snippets/repeatables/text.liquid
    transforms:
      - type: remove_repeatable
        path: templates/page.liquid
        name: text
`))
	writeTestFile(t, filepath.Join(root, "nimbu.yml"), "site: old-site\ntheme: old-theme\n")
	writeTestFile(t, filepath.Join(root, "package.json"), "{}\n")
	writeTestFile(t, filepath.Join(root, "src", "index.ts"), "export {}\n")
	writeTestFile(t, filepath.Join(root, "src", "recipes", "filepond", "alpine.ts"), "export default function AlpineFilePond() {}\n")
	writeTestFile(t, filepath.Join(root, "src", "recipes", "filepond", "index.css"), ".filepond {}\n")
	writeTestFile(t, filepath.Join(root, "build", "recipes", "filepond.ts"), "export default { name: 'filepond' };\n")
	writeTestFile(t, filepath.Join(root, "templates", "page.liquid"), `<main>
  {% repeatable "header", label: "Header" %}
    {% include "repeatables/header" %}
  {% endrepeatable %}

  {% repeatable "text", label: "Text" %}
    {% include "repeatables/text" %}
  {% endrepeatable %}
</main>
`)
	writeTestFile(t, filepath.Join(root, "templates", "customers", "login.liquid"), "login\n")
	writeTestFile(t, filepath.Join(root, "snippets", "repeatables", "header.liquid"), "header\n")
	writeTestFile(t, filepath.Join(root, "snippets", "repeatables", "text.liquid"), "text\n")
	// Files that should be excluded
	writeTestFile(t, filepath.Join(root, "README.md"), "# Starterskit\n")
	writeTestFile(t, filepath.Join(root, "AGENTS.md"), "agents\n")
	writeTestFile(t, filepath.Join(root, ".github", "ci.yml"), "on: push\n")

	initGitFixture(t, root)
}

func initGitFixture(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"add", "."},
		{"-c", "user.name=test", "-c", "user.email=test@test", "commit", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}
