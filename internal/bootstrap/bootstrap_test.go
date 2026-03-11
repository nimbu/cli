package bootstrap

import (
	"os"
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
