package apps

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	requirePattern = regexp.MustCompile(`require\(["']((?:\.\.?/)?[^"']+)["']\)`)
	importPattern  = regexp.MustCompile(`(?m)(?:^|\n)\s*import(?:[\s\w{},*]+from\s+)?["']((?:\.\.?/)?[^"']+)["']`)
	exportPattern  = regexp.MustCompile(`(?m)(?:^|\n)\s*export[\s\w{},*]*from\s+["']((?:\.\.?/)?[^"']+)["']`)
)

// OrderFiles topologically orders files by local import/require dependencies.
func OrderFiles(app AppConfig, files []string, provided ...map[string]string) ([]string, error) {
	if len(files) == 0 {
		return nil, nil
	}

	nameToFile := make(map[string]string, len(files))
	deps := make(map[string][]string, len(files))
	for _, file := range files {
		nameToFile[RemoteName(app, file)] = file
	}

	var contentMap map[string]string
	if len(provided) > 0 {
		contentMap = provided[0]
	}

	for _, file := range files {
		data := []byte(nil)
		if contentMap != nil {
			data = []byte(contentMap[file])
		} else {
			loaded, err := os.ReadFile(filepath.Join(app.ProjectRoot, filepath.FromSlash(file)))
			if err != nil {
				return nil, err
			}
			data = loaded
		}
		name := RemoteName(app, file)
		dependencies := dependencyNames(name, string(data))
		for _, dep := range dependencies {
			if _, ok := nameToFile[dep]; ok {
				deps[name] = append(deps[name], dep)
			}
		}
		sort.Strings(deps[name])
	}

	order, err := topoSort(nameToFile, deps)
	if err != nil {
		return nil, err
	}
	ordered := make([]string, 0, len(order))
	for _, name := range order {
		ordered = append(ordered, nameToFile[name])
	}
	return ordered, nil
}

func dependencyNames(name, source string) []string {
	found := map[string]struct{}{}
	for _, pattern := range []*regexp.Regexp{requirePattern, importPattern, exportPattern} {
		matches := pattern.FindAllStringSubmatch(source, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			if dep, ok := normalizeDependency(name, match[1]); ok {
				found[dep] = struct{}{}
			}
		}
	}
	deps := make([]string, 0, len(found))
	for dep := range found {
		deps = append(deps, dep)
	}
	sort.Strings(deps)
	return deps
}

func normalizeDependency(currentName, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, ".") {
		return "", false
	}
	base := path.Dir(currentName)
	if base == "." {
		base = ""
	}
	joined := path.Clean(path.Join(base, raw))
	switch path.Ext(joined) {
	case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx":
	default:
		joined += ".js"
	}
	return normalizePath(joined), true
}

func topoSort(nameToFile map[string]string, deps map[string][]string) ([]string, error) {
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var stack []string
	var order []string

	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			cycle := append([]string{}, stack...)
			cycle = append(cycle, name)
			return fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " -> "))
		}

		visiting[name] = true
		stack = append(stack, name)
		for _, dep := range deps[name] {
			if _, ok := nameToFile[dep]; !ok {
				continue
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		visiting[name] = false
		visited[name] = true
		order = append(order, name)
		return nil
	}

	names := make([]string, 0, len(nameToFile))
	for name := range nameToFile {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return order, nil
}
