package themes

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	projectconfig "github.com/nimbu/cli/internal/config"
)

var defaultGeneratedPatterns = []string{
	"javascripts/**",
	"stylesheets/**",
	"snippets/webpack_*.liquid",
}

var defaultRootPaths = map[Kind][]string{
	KindLayout:   {"layouts"},
	KindTemplate: {"templates"},
	KindSnippet:  {"snippets"},
	KindAsset:    {"images", "fonts", "javascripts", "stylesheets"},
}

// ResolveConfig resolves theme sync configuration from project config and flags.
func ResolveConfig(projectRoot string, project projectconfig.ProjectConfig, explicitTheme string) (Config, error) {
	theme := strings.TrimSpace(explicitTheme)
	if theme == "" {
		theme = strings.TrimSpace(project.Theme)
	}
	if theme == "" {
		return Config{}, fmt.Errorf("theme required; set theme in nimbu.yml or pass --theme")
	}

	cfg := Config{
		ProjectRoot: projectRoot,
		Theme:       theme,
		Generated:   append([]string{}, defaultGeneratedPatterns...),
	}

	roots := projectconfig.SyncRootsConfig{}
	if project.Sync != nil {
		if len(project.Sync.Generated) > 0 {
			cfg.Generated = append([]string{}, project.Sync.Generated...)
		}
		cfg.Ignore = append([]string{}, project.Sync.Ignore...)
		roots = project.Sync.Roots
		if build := resolveBuildConfig(projectRoot, project.Sync.Build); build != nil {
			cfg.Build = build
		}
	}

	resolvedRoots, err := resolveRoots(projectRoot, roots)
	if err != nil {
		return Config{}, err
	}
	cfg.Roots = resolvedRoots
	cfg.Generated = normalizePatterns(cfg.Generated)
	cfg.Ignore = normalizePatterns(cfg.Ignore)
	return cfg, nil
}

func resolveBuildConfig(projectRoot string, raw projectconfig.SyncBuildConfig) *BuildConfig {
	command := strings.TrimSpace(raw.Command)
	if command == "" {
		return nil
	}

	cwd := projectRoot
	if strings.TrimSpace(raw.CWD) != "" {
		cwd = resolveProjectPath(projectRoot, raw.CWD)
	}

	return &BuildConfig{
		Args:    append([]string{}, raw.Args...),
		CWD:     cwd,
		Command: command,
		Env:     cloneEnv(raw.Env),
	}
}

func resolveRoots(projectRoot string, raw projectconfig.SyncRootsConfig) ([]RootSpec, error) {
	byKind := map[Kind][]string{
		KindLayout:   effectiveRoots(raw.Layouts, defaultRootPaths[KindLayout]),
		KindTemplate: effectiveRoots(raw.Templates, defaultRootPaths[KindTemplate]),
		KindSnippet:  effectiveRoots(raw.Snippets, defaultRootPaths[KindSnippet]),
		KindAsset:    effectiveRoots(raw.Assets, defaultRootPaths[KindAsset]),
	}

	var roots []RootSpec
	seen := map[string]struct{}{}
	for _, kind := range []Kind{KindLayout, KindTemplate, KindSnippet, KindAsset} {
		for _, item := range byKind[kind] {
			localPath := normalizePath(strings.TrimSpace(item))
			if localPath == "" {
				continue
			}
			if err := validateRootPath(localPath); err != nil {
				return nil, fmt.Errorf("invalid sync root %q: %w", item, err)
			}
			absPath := resolveProjectPath(projectRoot, item)
			projectRel, err := projectRelativePath(projectRoot, absPath)
			if err != nil {
				return nil, fmt.Errorf("sync root %q must stay inside project root", item)
			}
			projectRel = normalizePath(projectRel)
			if projectRel == "." {
				projectRel = ""
			}
			key := string(kind) + ":" + projectRel
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			if _, err := os.Stat(absPath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("stat sync root %q: %w", item, err)
			}

			root := RootSpec{
				AbsPath:   absPath,
				Kind:      kind,
				LocalPath: projectRel,
			}
			if kind == KindAsset {
				root.RemoteBase = projectRel
			}
			roots = append(roots, root)
		}
	}

	return roots, nil
}

func effectiveRoots(values []string, defaults []string) []string {
	if len(values) == 0 {
		return defaults
	}
	return values
}

func validateRootPath(localPath string) error {
	clean := strings.Trim(localPath, "/")
	if clean == "" {
		return nil
	}
	first := clean
	if idx := strings.IndexByte(clean, '/'); idx >= 0 {
		first = clean[:idx]
	}
	switch first {
	case "code", "content":
		return fmt.Errorf("%s is not a theme resource root", first)
	default:
		return nil
	}
}

func resolveProjectPath(projectRoot string, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return projectRoot
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	return filepath.Clean(filepath.Join(projectRoot, trimmed))
}

func projectRelativePath(projectRoot, absPath string) (string, error) {
	rel, err := filepath.Rel(projectRoot, absPath)
	if err != nil {
		return "", err
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("outside project root")
	}
	return rel, nil
}

func normalizePath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = path.Clean(strings.TrimSpace(value))
	if value == "." {
		return "."
	}
	return strings.TrimPrefix(value, "./")
}

func normalizePatterns(patterns []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		normalized := normalizePath(pattern)
		if normalized == "" || normalized == "." {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func cloneEnv(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
