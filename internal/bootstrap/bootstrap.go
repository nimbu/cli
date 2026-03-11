package bootstrap

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const ManifestPath = "bootstrap/manifest.yml"

type Manifest struct {
	Name          string       `yaml:"name,omitempty"`
	BasePaths     []string     `yaml:"base_paths"`
	GeneratedDirs []string     `yaml:"generated_dirs,omitempty"`
	Bundles       []Bundle     `yaml:"bundles"`
	Recipes       []Recipe     `yaml:"recipes"`
	Repeatables   []Repeatable `yaml:"repeatables"`
}

type Bundle struct {
	ID          string      `yaml:"id"`
	Label       string      `yaml:"label"`
	Description string      `yaml:"description,omitempty"`
	Paths       []string    `yaml:"paths"`
	Recipes     []string    `yaml:"recipes,omitempty"`
	Transforms  []Transform `yaml:"transforms,omitempty"`
}

type Repeatable struct {
	ID          string      `yaml:"id"`
	Label       string      `yaml:"label"`
	Description string      `yaml:"description,omitempty"`
	Paths       []string    `yaml:"paths"`
	Recipes     []string    `yaml:"recipes,omitempty"`
	Transforms  []Transform `yaml:"transforms,omitempty"`
}

type Recipe struct {
	ID                string `yaml:"id"`
	RuntimeIdentifier string `yaml:"runtime_identifier,omitempty"`
	RuntimeImport     string `yaml:"runtime_import,omitempty"`
	RuntimeRegisterAs string `yaml:"runtime_register_as,omitempty"`
	CSSImport         string `yaml:"css_import,omitempty"`
	BuildIdentifier   string `yaml:"build_identifier,omitempty"`
	BuildImport       string `yaml:"build_import,omitempty"`
}

type Transform struct {
	Type  string `yaml:"type"`
	Path  string `yaml:"path"`
	Name  string `yaml:"name,omitempty"`
	Match string `yaml:"match,omitempty"`
}

type BootstrapOptions struct {
	Manifest       Manifest
	SourceDir      string
	DestinationDir string
	Site           string
	Theme          string
	BundleIDs      []string
	RepeatableIDs  []string
}

type Result struct {
	Path        string   `json:"path"`
	Site        string   `json:"site"`
	Theme       string   `json:"theme"`
	Bundles     []string `json:"bundles,omitempty"`
	Repeatables []string `json:"repeatables,omitempty"`
}

func LoadManifest(root string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, ManifestPath))
	if err != nil {
		return Manifest{}, fmt.Errorf("read bootstrap manifest: %w", err)
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse bootstrap manifest: %w", err)
	}
	if err := manifest.validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func BootstrapProject(opts BootstrapOptions) (Result, error) {
	if strings.TrimSpace(opts.SourceDir) == "" {
		return Result{}, fmt.Errorf("source dir required")
	}
	if strings.TrimSpace(opts.DestinationDir) == "" {
		return Result{}, fmt.Errorf("destination dir required")
	}
	if strings.TrimSpace(opts.Site) == "" {
		return Result{}, fmt.Errorf("site required")
	}
	if strings.TrimSpace(opts.Theme) == "" {
		return Result{}, fmt.Errorf("theme required")
	}
	if err := opts.Manifest.validate(); err != nil {
		return Result{}, err
	}

	destDir := filepath.Clean(opts.DestinationDir)
	if _, err := os.Stat(destDir); err == nil {
		return Result{}, fmt.Errorf("destination already exists: %s", destDir)
	} else if !os.IsNotExist(err) {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return Result{}, fmt.Errorf("create output dir: %w", err)
	}

	selectedBundles, selectedRepeatables, recipeSet, transformList, pathSet, err := opts.Manifest.resolve(opts.BundleIDs, opts.RepeatableIDs)
	if err != nil {
		return Result{}, err
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create destination: %w", err)
	}
	for _, rel := range sortedKeys(pathSet) {
		if err := copyPath(filepath.Join(opts.SourceDir, filepath.FromSlash(rel)), filepath.Join(destDir, filepath.FromSlash(rel))); err != nil {
			if shouldSkipMissingBootstrapPath(rel, err) {
				continue
			}
			return Result{}, err
		}
	}
	if err := materializeGeneratedDirs(destDir, opts.Manifest.GeneratedDirs); err != nil {
		return Result{}, err
	}

	if err := applyTransforms(destDir, transformList); err != nil {
		return Result{}, err
	}
	if err := writeRecipeIndexes(destDir, opts.Manifest, recipeSet); err != nil {
		return Result{}, err
	}
	if err := rewriteProjectConfig(filepath.Join(destDir, "nimbu.yml"), opts.Site, opts.Theme); err != nil {
		return Result{}, err
	}
	if err := initGitRepo(destDir); err != nil {
		return Result{}, err
	}

	return Result{
		Path:        destDir,
		Site:        opts.Site,
		Theme:       opts.Theme,
		Bundles:     selectedBundles,
		Repeatables: selectedRepeatables,
	}, nil
}

func (m Manifest) resolve(bundleIDs, repeatableIDs []string) ([]string, []string, map[string]Recipe, []Transform, map[string]struct{}, error) {
	bundleMap := make(map[string]Bundle, len(m.Bundles))
	for _, bundle := range m.Bundles {
		bundleMap[bundle.ID] = bundle
	}
	repeatableMap := make(map[string]Repeatable, len(m.Repeatables))
	for _, repeatable := range m.Repeatables {
		repeatableMap[repeatable.ID] = repeatable
	}
	recipeMap := m.recipeMap()

	pathSet := make(map[string]struct{}, len(m.BasePaths))
	for _, path := range m.BasePaths {
		pathSet[path] = struct{}{}
	}

	selectedBundleIDs := uniqueStrings(bundleIDs)
	selectedRepeatableIDs := uniqueStrings(repeatableIDs)
	selectedRecipes := map[string]Recipe{}
	var transforms []Transform

	for _, id := range selectedBundleIDs {
		bundle, ok := bundleMap[id]
		if !ok {
			return nil, nil, nil, nil, nil, fmt.Errorf("unknown bundle %q", id)
		}
		for _, path := range bundle.Paths {
			pathSet[path] = struct{}{}
		}
		for _, recipeID := range bundle.Recipes {
			selectedRecipes[recipeID] = recipeMap[recipeID]
		}
	}

	selectedRepeatableSet := make(map[string]struct{}, len(selectedRepeatableIDs))
	for _, id := range selectedRepeatableIDs {
		repeatable, ok := repeatableMap[id]
		if !ok {
			return nil, nil, nil, nil, nil, fmt.Errorf("unknown repeatable %q", id)
		}
		selectedRepeatableSet[id] = struct{}{}
		for _, path := range repeatable.Paths {
			pathSet[path] = struct{}{}
		}
		for _, recipeID := range repeatable.Recipes {
			selectedRecipes[recipeID] = recipeMap[recipeID]
		}
	}

	for _, repeatable := range m.Repeatables {
		if _, ok := selectedRepeatableSet[repeatable.ID]; ok {
			continue
		}
		transforms = append(transforms, repeatable.Transforms...)
	}

	return selectedBundleIDs, selectedRepeatableIDs, selectedRecipes, transforms, pathSet, nil
}

func (m Manifest) validate() error {
	if len(m.BasePaths) == 0 {
		return fmt.Errorf("bootstrap manifest missing base_paths")
	}
	for _, path := range m.GeneratedDirs {
		if err := validateManifestPath(path); err != nil {
			return err
		}
	}

	recipes := make(map[string]Recipe, len(m.Recipes))
	for _, recipe := range m.Recipes {
		if strings.TrimSpace(recipe.ID) == "" {
			return fmt.Errorf("recipe id required")
		}
		recipes[recipe.ID] = recipe
	}
	if err := validateEntries(m.Bundles, recipes); err != nil {
		return err
	}
	if err := validateEntries(m.Repeatables, recipes); err != nil {
		return err
	}
	return nil
}

func validateEntries[T interface {
	entryID() string
	entryPaths() []string
	entryRecipes() []string
	entryTransforms() []Transform
}](entries []T, recipes map[string]Recipe) error {
	seen := map[string]struct{}{}
	for _, entry := range entries {
		id := strings.TrimSpace(entry.entryID())
		if id == "" {
			return fmt.Errorf("manifest entry id required")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate manifest entry id %q", id)
		}
		seen[id] = struct{}{}
		for _, path := range entry.entryPaths() {
			if err := validateManifestPath(path); err != nil {
				return err
			}
		}
		for _, recipeID := range entry.entryRecipes() {
			if _, ok := recipes[recipeID]; !ok {
				return fmt.Errorf("unknown recipe %q", recipeID)
			}
		}
		for _, transform := range entry.entryTransforms() {
			if err := validateTransform(transform); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateManifestPath(path string) error {
	clean := filepath.ToSlash(filepath.Clean(path))
	switch {
	case strings.TrimSpace(path) == "":
		return fmt.Errorf("manifest path required")
	case strings.HasPrefix(clean, "../"), clean == "..", filepath.IsAbs(path):
		return fmt.Errorf("manifest path must stay inside source root: %s", path)
	default:
		return nil
	}
}

func validateTransform(transform Transform) error {
	if err := validateManifestPath(transform.Path); err != nil {
		return err
	}
	switch transform.Type {
	case "remove_repeatable":
		if strings.TrimSpace(transform.Name) == "" {
			return fmt.Errorf("remove_repeatable requires name")
		}
	case "remove_exact":
		if strings.TrimSpace(transform.Match) == "" {
			return fmt.Errorf("remove_exact requires match")
		}
	case "remove_file":
	default:
		return fmt.Errorf("unsupported transform type %q", transform.Type)
	}
	return nil
}

func (m Manifest) recipeMap() map[string]Recipe {
	out := make(map[string]Recipe, len(m.Recipes))
	for _, recipe := range m.Recipes {
		out[recipe.ID] = recipe
	}
	return out
}

func copyPath(sourcePath, destPath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("stat source %s: %w", sourcePath, err)
	}
	if info.IsDir() {
		return filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(sourcePath, path)
			if err != nil {
				return err
			}
			target := filepath.Join(destPath, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			return copyFile(path, target)
		})
	}
	return copyFile(sourcePath, destPath)
}

func copyFile(sourcePath, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", destPath, err)
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source %s: %w", sourcePath, err)
	}
	defer func() { _ = sourceFile.Close() }()

	info, err := sourceFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source %s: %w", sourcePath, err)
	}

	destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
	if err != nil {
		return fmt.Errorf("open destination %s: %w", destPath, err)
	}
	defer func() { _ = destFile.Close() }()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("copy %s: %w", sourcePath, err)
	}
	return nil
}

func shouldSkipMissingBootstrapPath(rel string, err error) bool {
	if !errors.Is(err, os.ErrNotExist) {
		return false
	}

	switch filepath.ToSlash(filepath.Clean(rel)) {
	case "javascripts", "stylesheets", "scripts", "snippets/webpack_app.liquid":
		return true
	default:
		return false
	}
}

func materializeGeneratedDirs(root string, dirs []string) error {
	for _, dir := range dirs {
		targetDir := filepath.Join(root, filepath.FromSlash(dir))
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return fmt.Errorf("create generated dir %s: %w", dir, err)
		}

		gitkeepPath := filepath.Join(targetDir, ".gitkeep")
		if _, err := os.Stat(gitkeepPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat generated dir marker %s: %w", dir, err)
		}

		if err := os.WriteFile(gitkeepPath, []byte{}, 0o644); err != nil {
			return fmt.Errorf("write generated dir marker %s: %w", dir, err)
		}
	}
	return nil
}

func applyTransforms(root string, transforms []Transform) error {
	for _, transform := range transforms {
		targetPath := filepath.Join(root, filepath.FromSlash(transform.Path))
		switch transform.Type {
		case "remove_file":
			if err := os.RemoveAll(targetPath); err != nil {
				return fmt.Errorf("remove file %s: %w", transform.Path, err)
			}
		case "remove_exact":
			data, err := os.ReadFile(targetPath)
			if err != nil {
				return fmt.Errorf("read transform target %s: %w", transform.Path, err)
			}
			contents := string(data)
			if !strings.Contains(contents, transform.Match) {
				return fmt.Errorf("manifest drift: %s missing match for transform", transform.Path)
			}
			contents = strings.Replace(contents, transform.Match, "", 1)
			if err := os.WriteFile(targetPath, []byte(contents), 0o644); err != nil {
				return fmt.Errorf("write transform target %s: %w", transform.Path, err)
			}
		case "remove_repeatable":
			if err := removeRepeatableBlock(targetPath, transform.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeRepeatableBlock(path string, name string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read transform target %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	start := -1
	depth := 0
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, `{% repeatable "`) || strings.Contains(trimmed, "{% repeatable '") {
			if start == -1 && (strings.Contains(trimmed, `"`+name+`"`) || strings.Contains(trimmed, `'`+name+`'`)) {
				start = idx
			}
			if start != -1 {
				depth++
			}
			continue
		}
		if start != -1 && strings.Contains(trimmed, "{% endrepeatable %}") {
			depth--
			if depth == 0 {
				lines = append(lines[:start], lines[idx+1:]...)
				output := strings.TrimLeft(strings.Join(lines, "\n"), "\n")
				return os.WriteFile(path, []byte(output), 0o644)
			}
		}
	}
	return fmt.Errorf("manifest drift: repeatable %q not found in %s", name, filepath.ToSlash(path))
}

func writeRecipeIndexes(root string, manifest Manifest, selected map[string]Recipe) error {
	recipes := make([]Recipe, 0, len(selected))
	for _, manifestRecipe := range manifest.Recipes {
		if recipe, ok := selected[manifestRecipe.ID]; ok {
			recipes = append(recipes, recipe)
		}
	}

	if err := writeFile(root, "src/recipes/index.ts", renderRuntimeRecipeIndex(recipes)); err != nil {
		return err
	}
	if err := writeFile(root, "src/recipes/index.css", renderCSSRecipeIndex(recipes)); err != nil {
		return err
	}
	if err := writeFile(root, "build/recipes/index.ts", renderBuildRecipeIndex(recipes)); err != nil {
		return err
	}
	return nil
}

func renderRuntimeRecipeIndex(recipes []Recipe) string {
	var buf bytes.Buffer
	for _, recipe := range recipes {
		if recipe.RuntimeIdentifier == "" || recipe.RuntimeImport == "" || recipe.RuntimeRegisterAs == "" {
			continue
		}
		fmt.Fprintf(&buf, "import %s from '%s';\n", recipe.RuntimeIdentifier, recipe.RuntimeImport)
	}
	if buf.Len() > 0 {
		buf.WriteByte('\n')
	}
	buf.WriteString("interface AlpineRegistry {\n\tdata(name: string, callback: unknown): void;\n}\n\n")
	buf.WriteString("export function registerRecipeComponents(alpine: AlpineRegistry) {\n")
	for _, recipe := range recipes {
		if recipe.RuntimeIdentifier == "" || recipe.RuntimeRegisterAs == "" {
			continue
		}
		fmt.Fprintf(&buf, "\talpine.data('%s', %s);\n", recipe.RuntimeRegisterAs, recipe.RuntimeIdentifier)
	}
	buf.WriteString("}\n")
	return buf.String()
}

func renderCSSRecipeIndex(recipes []Recipe) string {
	var imports []string
	for _, recipe := range recipes {
		if recipe.CSSImport != "" {
			imports = append(imports, fmt.Sprintf("@import '%s';", recipe.CSSImport))
		}
	}
	if len(imports) == 0 {
		return ""
	}
	return strings.Join(imports, "\n") + "\n"
}

func renderBuildRecipeIndex(recipes []Recipe) string {
	var buf bytes.Buffer
	var identifiers []string
	for _, recipe := range recipes {
		if recipe.BuildIdentifier == "" || recipe.BuildImport == "" {
			continue
		}
		fmt.Fprintf(&buf, "import %s from '%s';\n", recipe.BuildIdentifier, recipe.BuildImport)
		identifiers = append(identifiers, recipe.BuildIdentifier)
	}
	if len(identifiers) == 0 {
		return "export default [];\n"
	}
	buf.WriteByte('\n')
	fmt.Fprintf(&buf, "export default [%s];\n", strings.Join(identifiers, ", "))
	return buf.String()
}

func rewriteProjectConfig(path, site, theme string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read project config %s: %w", path, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse project config %s: %w", path, err)
	}
	if len(root.Content) == 0 {
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	body := root.Content[0]
	setMappingScalar(body, "site", site)
	setMappingScalar(body, "theme", theme)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return fmt.Errorf("encode project config %s: %w", path, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("close project config encoder %s: %w", path, err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func setMappingScalar(node *yaml.Node, key, value string) {
	for idx := 0; idx+1 < len(node.Content); idx += 2 {
		if node.Content[idx].Value == key {
			node.Content[idx+1].Kind = yaml.ScalarNode
			node.Content[idx+1].Tag = "!!str"
			node.Content[idx+1].Value = value
			return
		}
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

func initGitRepo(root string) error {
	if err := runGit(root, "init"); err != nil {
		return err
	}
	if err := runGit(root, "add", "."); err != nil {
		return err
	}
	if err := runGit(root,
		"-c", "user.name=nimbu",
		"-c", "user.email=noreply@nimbu.local",
		"commit", "-m", "chore: bootstrap theme project",
	); err != nil {
		return err
	}
	return nil
}

func runGit(root string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func writeFile(root, relPath, contents string) error {
	target := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	return nil
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func (b Bundle) entryID() string              { return b.ID }
func (b Bundle) entryPaths() []string         { return b.Paths }
func (b Bundle) entryRecipes() []string       { return b.Recipes }
func (b Bundle) entryTransforms() []Transform { return b.Transforms }

func (r Repeatable) entryID() string              { return r.ID }
func (r Repeatable) entryPaths() []string         { return r.Paths }
func (r Repeatable) entryRecipes() []string       { return r.Recipes }
func (r Repeatable) entryTransforms() []Transform { return r.Transforms }
