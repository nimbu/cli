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
	selectors        []themeSelector
	hasCategory      bool
	liquidOnly       bool
	categoryMatchers map[string][]globMatcher
}

type selectorKind int

const (
	selectorPath selectorKind = iota
	selectorGlob
)

type themeSelector struct {
	raw     string
	pattern string
	kind    selectorKind
	matcher globMatcher
}

func compileSelectionFilter(cfg Config, opts Options) (*selectionFilter, error) {
	selectors, err := onlySelectors(opts)
	if err != nil {
		return nil, err
	}

	filter := &selectionFilter{
		cfg:              cfg,
		selectors:        selectors,
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
	onlyMatch := ok && f.matchesOnly(projectPath)
	if len(f.selectors) > 0 && !f.hasCategory {
		return onlyMatch
	}
	if !f.hasCategory {
		return true
	}
	categoryMatch := matchesResourceCategory(resource, projectPath, f.categoryMatchers, f.liquidOnly)
	if len(f.selectors) > 0 {
		return onlyMatch || categoryMatch
	}
	return categoryMatch
}

// FilterResources applies selection flags to resources.
func FilterResources(cfg Config, resources []Resource, opts Options) ([]Resource, error) {
	filter, err := compileSelectionFilter(cfg, opts)
	if err != nil {
		return nil, err
	}
	if err := filter.validateOnlySelectors(resources); err != nil {
		return nil, err
	}

	filtered := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		if !filter.Match(resource) {
			continue
		}
		filtered = append(filtered, resource)
	}
	return filtered, nil
}

func onlySelectors(opts Options) ([]themeSelector, error) {
	if len(opts.Only) == 0 {
		return nil, nil
	}
	selectors := make([]themeSelector, 0, len(opts.Only))
	for _, value := range opts.Only {
		selector, ok, err := compileThemeSelector(value)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		selectors = append(selectors, selector)
	}
	if len(selectors) == 0 {
		return nil, nil
	}
	return selectors, nil
}

func compileThemeSelector(value string) (themeSelector, bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return themeSelector{}, false, nil
	}
	if isAbsoluteThemePathInput(trimmed) {
		return themeSelector{}, false, fmt.Errorf("theme selector must be project-relative: %s", value)
	}
	normalized := normalizePath(trimmed)
	if normalized == "." || normalized == "" {
		return themeSelector{}, false, fmt.Errorf("invalid theme selector: %s", value)
	}
	if strings.HasPrefix(normalized, "../") || normalized == ".." {
		return themeSelector{}, false, fmt.Errorf("theme selector must stay inside project root: %s", value)
	}

	selector := themeSelector{raw: value, pattern: normalized, kind: selectorPath}
	if strings.ContainsAny(normalized, "*?") {
		matcher, err := compileMatcher(normalized)
		if err != nil {
			return themeSelector{}, false, err
		}
		selector.kind = selectorGlob
		selector.matcher = matcher
	}
	return selector, true, nil
}

func compileMatcher(pattern string) (globMatcher, error) {
	re, err := globToRegexp(pattern)
	if err != nil {
		return globMatcher{}, err
	}
	return globMatcher{pattern: pattern, re: re}, nil
}

func (f *selectionFilter) matchesOnly(projectPath string) bool {
	for _, selector := range f.selectors {
		if selector.Match(projectPath) {
			return true
		}
	}
	return false
}

func (f *selectionFilter) validateOnlySelectors(resources []Resource) error {
	for _, selector := range f.selectors {
		matched := false
		for _, resource := range resources {
			projectPath, ok := projectPathForResource(f.cfg, resource)
			if !ok {
				continue
			}
			if selector.Match(projectPath) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("theme selector matched no managed files: %s", selector.raw)
		}
	}
	return nil
}

func (s themeSelector) Match(projectPath string) bool {
	projectPath = normalizePath(projectPath)
	switch s.kind {
	case selectorGlob:
		return s.matcher.re.MatchString(projectPath)
	default:
		return projectPath == s.pattern || hasPathPrefix(projectPath, s.pattern)
	}
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
	if isAbsoluteThemePathInput(trimmed) {
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

func isAbsoluteThemePathInput(value string) bool {
	slashed := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	return filepath.IsAbs(value) ||
		strings.HasPrefix(slashed, "/") ||
		hasWindowsDrivePrefix(slashed)
}

func hasWindowsDrivePrefix(value string) bool {
	return len(value) >= 2 && value[1] == ':'
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
