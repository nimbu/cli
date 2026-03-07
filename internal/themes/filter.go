package themes

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

var (
	cssPatterns   = []string{"**/*.css", "**/*.scss", "**/*.sass", "**/*.less"}
	jsPatterns    = []string{"**/*.js", "**/*.mjs", "**/*.cjs", "**/*.jsx", "**/*.ts", "**/*.tsx"}
	imagePatterns = []string{"**/*.png", "**/*.jpg", "**/*.jpeg", "**/*.gif", "**/*.webp", "**/*.svg", "**/*.avif", "**/*.ico"}
	fontPatterns  = []string{"**/*.woff", "**/*.woff2", "**/*.ttf", "**/*.otf", "**/*.eot"}
)

type selectionFilter struct {
	cfg              Config
	onlySet          map[string]struct{}
	hasCategory      bool
	liquidOnly       bool
	categoryMatchers map[string][]globMatcher
}

func compileSelectionFilter(cfg Config, opts Options) (*selectionFilter, error) {
	onlySet, err := explicitOnlySet(cfg, opts.Only)
	if err != nil {
		return nil, err
	}

	filter := &selectionFilter{
		cfg:              cfg,
		onlySet:          onlySet,
		liquidOnly:       opts.LiquidOnly,
		categoryMatchers: map[string][]globMatcher{},
	}
	addCategory := func(name string, patterns []string) error {
		matchers, err := compileMatchers(patterns)
		if err != nil {
			return err
		}
		filter.hasCategory = true
		filter.categoryMatchers[name] = matchers
		return nil
	}
	if opts.CSSOnly {
		if err := addCategory("css", cssPatterns); err != nil {
			return nil, err
		}
	}
	if opts.JSOnly {
		if err := addCategory("js", jsPatterns); err != nil {
			return nil, err
		}
	}
	if opts.ImagesOnly {
		if err := addCategory("images", imagePatterns); err != nil {
			return nil, err
		}
	}
	if opts.FontsOnly {
		if err := addCategory("fonts", fontPatterns); err != nil {
			return nil, err
		}
	}
	if opts.LiquidOnly {
		filter.hasCategory = true
	}
	return filter, nil
}

func (f *selectionFilter) Match(resource Resource) bool {
	if f == nil {
		return true
	}
	projectPath, ok := projectPathForResource(f.cfg, resource)
	if len(f.onlySet) > 0 {
		if !ok {
			return false
		}
		if _, exists := f.onlySet[projectPath]; !exists {
			return false
		}
	}
	if !f.hasCategory {
		return true
	}
	return matchesResourceCategory(resource, projectPath, f.categoryMatchers, f.liquidOnly)
}

// FilterResources applies selection flags to resources.
func FilterResources(cfg Config, resources []Resource, opts Options) ([]Resource, error) {
	onlySet, err := explicitOnlySet(cfg, opts.Only)
	if err != nil {
		return nil, err
	}

	var hasCategory bool
	categoryMatchers := map[string][]globMatcher{}
	if opts.CSSOnly {
		hasCategory = true
		matchers, err := compileMatchers(cssPatterns)
		if err != nil {
			return nil, err
		}
		categoryMatchers["css"] = matchers
	}
	if opts.JSOnly {
		hasCategory = true
		matchers, err := compileMatchers(jsPatterns)
		if err != nil {
			return nil, err
		}
		categoryMatchers["js"] = matchers
	}
	if opts.ImagesOnly {
		hasCategory = true
		matchers, err := compileMatchers(imagePatterns)
		if err != nil {
			return nil, err
		}
		categoryMatchers["images"] = matchers
	}
	if opts.FontsOnly {
		hasCategory = true
		matchers, err := compileMatchers(fontPatterns)
		if err != nil {
			return nil, err
		}
		categoryMatchers["fonts"] = matchers
	}
	if opts.LiquidOnly {
		hasCategory = true
	}

	if len(onlySet) == 0 && !hasCategory {
		return append([]Resource{}, resources...), nil
	}

	filtered := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		projectPath, ok := projectPathForResource(cfg, resource)
		if len(onlySet) > 0 {
			if !ok {
				continue
			}
			if _, exists := onlySet[projectPath]; !exists {
				continue
			}
		}
		if hasCategory && !matchesResourceCategory(resource, projectPath, categoryMatchers, opts.LiquidOnly) {
			continue
		}
		filtered = append(filtered, resource)
	}
	return filtered, nil
}

func explicitOnlySet(cfg Config, values []string) (map[string]struct{}, error) {
	if len(values) == 0 {
		return nil, nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized, err := normalizeOnlyPath(cfg.ProjectRoot, value)
		if err != nil {
			return nil, err
		}
		if normalized == "" {
			continue
		}
		if _, ok, err := ClassifyProjectPath(cfg, normalized); err != nil {
			return nil, err
		} else if !ok {
			return nil, fmt.Errorf("--only path is not in a managed theme root: %s", value)
		}
		set[normalized] = struct{}{}
	}
	if len(set) == 0 {
		return nil, nil
	}
	return set, nil
}

func normalizeOnlyPath(projectRoot, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("--only path must be project-relative: %s", value)
	}
	normalized := normalizePath(trimmed)
	if normalized == "." || normalized == "" {
		return "", fmt.Errorf("invalid --only path: %s", value)
	}
	if strings.HasPrefix(normalized, "../") || normalized == ".." {
		return "", fmt.Errorf("--only path must stay inside project root: %s", value)
	}
	return normalized, nil
}

func matchesResourceCategory(resource Resource, projectPath string, matchers map[string][]globMatcher, liquidOnly bool) bool {
	if liquidOnly && resource.Kind != KindAsset {
		return true
	}
	if resource.Kind != KindAsset {
		return false
	}
	for _, kindMatchers := range matchers {
		if matchesAny(kindMatchers, projectPath) {
			return true
		}
	}
	return false
}

// ProjectPathForResource maps a remote resource back to a project-relative local path.
func ProjectPathForResource(cfg Config, resource Resource) (string, bool) {
	return projectPathForResource(cfg, resource)
}

func localPathForRemote(cfg Config, resource Resource) (string, bool) {
	return projectPathForResource(cfg, resource)
}

func projectPathForResource(cfg Config, resource Resource) (string, bool) {
	if resource.LocalPath != "" {
		return normalizePath(resource.LocalPath), true
	}

	switch resource.Kind {
	case KindAsset:
		best := RootSpec{}
		bestLen := -1
		for _, root := range cfg.Roots {
			if root.Kind != KindAsset {
				continue
			}
			base := normalizePath(root.RemoteBase)
			if base == "." {
				base = ""
			}
			if base != "" && !hasPathPrefix(resource.RemoteName, base) {
				continue
			}
			if len(base) > bestLen {
				best = root
				bestLen = len(base)
			}
			if base == "" && bestLen < 0 {
				best = root
				bestLen = 0
			}
		}
		if bestLen < 0 {
			return "", false
		}
		relative := resource.RemoteName
		base := normalizePath(best.RemoteBase)
		if base != "" && base != "." {
			relative = strings.TrimPrefix(relative, base)
			relative = strings.TrimPrefix(relative, "/")
		}
		if best.LocalPath == "" || best.LocalPath == "." {
			return normalizePath(relative), true
		}
		return normalizePath(path.Join(best.LocalPath, relative)), true
	default:
		for _, root := range cfg.Roots {
			if root.Kind != resource.Kind {
				continue
			}
			if root.LocalPath == "" || root.LocalPath == "." {
				return normalizePath(resource.RemoteName), true
			}
			return normalizePath(path.Join(root.LocalPath, resource.RemoteName)), true
		}
		return "", false
	}
}
