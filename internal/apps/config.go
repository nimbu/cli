package apps

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/config"
)

// AppConfig is one resolved local app config.
type AppConfig struct {
	config.AppProjectConfig
	ProjectRoot string
}

// VisibleApps filters apps by active host/site.
func VisibleApps(projectRoot string, project config.ProjectConfig, activeHost, activeSite string) []AppConfig {
	var visible []AppConfig
	for _, item := range project.Apps {
		if item.Host != "" && !strings.EqualFold(strings.TrimSpace(item.Host), strings.TrimSpace(activeHost)) {
			continue
		}
		if item.Site != "" && strings.TrimSpace(item.Site) != strings.TrimSpace(activeSite) {
			continue
		}
		visible = append(visible, AppConfig{AppProjectConfig: item, ProjectRoot: projectRoot})
	}
	sort.SliceStable(visible, func(i, j int) bool {
		return visible[i].Name < visible[j].Name
	})
	return visible
}

// ResolveApp selects one app by local name.
func ResolveApp(candidates []AppConfig, requested string) (AppConfig, error) {
	if requested != "" {
		for _, app := range candidates {
			if app.Name == requested {
				return app, nil
			}
		}
		return AppConfig{}, fmt.Errorf("configured app %q not found", requested)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) == 0 {
		return AppConfig{}, fmt.Errorf("no apps configured for current host/site; run 'nimbu-cli apps config'")
	}
	return AppConfig{}, fmt.Errorf("multiple apps configured; pass --app")
}

// CollectFiles returns all files under dir matching glob, project-root relative.
func CollectFiles(app AppConfig) ([]string, error) {
	dir := filepath.Join(app.ProjectRoot, filepath.FromSlash(strings.TrimSpace(app.Dir)))
	pattern := normalizePath(strings.TrimSpace(app.Glob))
	if pattern == "" {
		pattern = "**/*.js"
	}
	matcher, err := compileMatcher(pattern)
	if err != nil {
		return nil, err
	}

	var files []string
	err = filepath.WalkDir(dir, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && current != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(app.ProjectRoot, current)
		if err != nil {
			return err
		}
		rel = normalizePath(rel)
		dirPrefix := normalizePath(app.Dir)
		within := rel
		if dirPrefix != "" && dirPrefix != "." {
			within = strings.TrimPrefix(rel, dirPrefix)
			within = strings.TrimPrefix(within, "/")
		}
		if matcher(within) {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func RemoteName(app AppConfig, projectRel string) string {
	dirPrefix := normalizePath(app.Dir)
	name := normalizePath(projectRel)
	if dirPrefix != "" && dirPrefix != "." {
		name = strings.TrimPrefix(name, dirPrefix)
		name = strings.TrimPrefix(name, "/")
	}
	return name
}

type matcher func(string) bool

func compileMatcher(pattern string) (matcher, error) {
	pattern = normalizePath(pattern)
	if pattern == "" {
		return func(string) bool { return true }, nil
	}
	patterns := []string{pattern}
	if strings.HasPrefix(pattern, "**/") {
		patterns = append(patterns, strings.TrimPrefix(pattern, "**/"))
	}
	var regexps []*regexp.Regexp
	for _, item := range patterns {
		re, err := globToRegexp(item)
		if err != nil {
			return nil, err
		}
		regexps = append(regexps, re)
	}
	return func(value string) bool {
		normalized := normalizePath(value)
		for _, re := range regexps {
			if re.MatchString(normalized) {
				return true
			}
		}
		return false
	}, nil
}

func normalizePath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = path.Clean(strings.TrimSpace(value))
	if value == "." {
		return "."
	}
	return strings.TrimPrefix(value, "./")
}
