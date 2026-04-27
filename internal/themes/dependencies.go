package themes

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ResourceContent pairs a theme resource with the bytes that will be uploaded.
type ResourceContent struct {
	Resource Resource
	Content  []byte
}

var (
	liquidOpaqueBlockPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?is){%-?\s*comment\s*-?%}.*?{%-?\s*endcomment\s*-?%}`),
		regexp.MustCompile(`(?is){%-?\s*raw\s*-?%}.*?{%-?\s*endraw\s*-?%}`),
	}
	liquidDependencyTagPattern = regexp.MustCompile(`(?is){%-?\s*(include|layout)\s+([^%]*?)-?%}`)
	staticLiquidNamePattern    = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
)

// OrderResourceContentByLiquidDependencies returns resources ordered so Liquid
// dependencies are uploaded before the resources that reference them.
func OrderResourceContentByLiquidDependencies(items []ResourceContent) ([]ResourceContent, []string) {
	if len(items) == 0 {
		return nil, nil
	}

	orderedInput := append([]ResourceContent(nil), items...)
	sort.SliceStable(orderedInput, func(i, j int) bool {
		return resourceLess(orderedInput[i].Resource, orderedInput[j].Resource)
	})

	byKey := make(map[resourceKey]ResourceContent, len(orderedInput))
	for _, item := range orderedInput {
		byKey[keyFor(item.Resource)] = item
	}

	dependents := make(map[resourceKey][]resourceKey, len(orderedInput))
	indegree := make(map[resourceKey]int, len(orderedInput))
	var warnings []string
	for _, item := range orderedInput {
		currentKey := keyFor(item.Resource)
		indegree[currentKey] = 0
	}

	for _, item := range orderedInput {
		currentKey := keyFor(item.Resource)
		dependencies, dependencyWarnings := parseLiquidDependencies(item.Resource, item.Content)
		warnings = append(warnings, dependencyWarnings...)

		seen := map[resourceKey]bool{}
		for _, dependency := range dependencies {
			if seen[dependency] {
				continue
			}
			seen[dependency] = true
			if _, ok := byKey[dependency]; !ok {
				warnings = append(warnings, fmt.Sprintf("dependency not in transfer set: %s references %s", item.Resource.DisplayPath, dependency.DisplayPath()))
				continue
			}
			dependents[dependency] = append(dependents[dependency], currentKey)
			indegree[currentKey]++
		}
	}

	ready := make([]resourceKey, 0, len(orderedInput))
	for _, item := range orderedInput {
		key := keyFor(item.Resource)
		if indegree[key] == 0 {
			ready = append(ready, key)
		}
	}
	sortResourceKeys(ready, byKey)

	var ordered []ResourceContent
	visited := map[resourceKey]bool{}
	var cycleWarned bool
	for len(ordered) < len(orderedInput) {
		if len(ready) == 0 {
			var remaining []resourceKey
			for _, item := range orderedInput {
				key := keyFor(item.Resource)
				if !visited[key] {
					remaining = append(remaining, key)
				}
			}
			sortResourceKeys(remaining, byKey)
			if len(remaining) == 0 {
				break
			}
			if !cycleWarned {
				paths := make([]string, len(remaining))
				for i, key := range remaining {
					paths[i] = byKey[key].Resource.DisplayPath
				}
				warnings = append(warnings, fmt.Sprintf("cycle in liquid dependencies: %s", strings.Join(paths, ", ")))
				cycleWarned = true
			}
			ready = append(ready, remaining[0])
		}

		key := ready[0]
		ready = ready[1:]
		if visited[key] {
			continue
		}
		visited[key] = true
		ordered = append(ordered, byKey[key])

		sortResourceKeys(dependents[key], byKey)
		for _, dependent := range dependents[key] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
		sortResourceKeys(ready, byKey)
	}

	sort.Strings(warnings)
	return ordered, warnings
}

func parseLiquidDependencies(resource Resource, content []byte) ([]resourceKey, []string) {
	if resource.Kind == KindAsset {
		return nil, nil
	}

	code := stripLiquidOpaqueBlocks(string(content))
	matches := liquidDependencyTagPattern.FindAllStringSubmatch(code, -1)
	dependencies := make([]resourceKey, 0, len(matches))
	var warnings []string

	for _, match := range matches {
		tag := strings.ToLower(strings.TrimSpace(match[1]))
		arg, ok := firstLiquidTagArgument(match[2])
		if !ok {
			warnings = append(warnings, fmt.Sprintf("dynamic %s in %s: %s", tag, resource.DisplayPath, strings.TrimSpace(match[2])))
			continue
		}
		name := normalizeLiquidDependencyName(arg)
		if name == "" {
			continue
		}
		switch tag {
		case "include":
			dependencies = append(dependencies, resourceKey{kind: KindSnippet, remoteName: name})
		case "layout":
			dependencies = append(dependencies, resourceKey{kind: KindLayout, remoteName: name})
		}
	}

	sort.SliceStable(dependencies, func(i, j int) bool {
		return dependencies[i].remoteName < dependencies[j].remoteName
	})
	return dependencies, warnings
}

func stripLiquidOpaqueBlocks(code string) string {
	for _, pattern := range liquidOpaqueBlockPatterns {
		code = pattern.ReplaceAllString(code, "")
	}
	return code
}

func firstLiquidTagArgument(markup string) (string, bool) {
	value := strings.TrimSpace(markup)
	if value == "" {
		return "", false
	}
	if value[0] == '"' || value[0] == '\'' {
		quote := value[0]
		for idx := 1; idx < len(value); idx++ {
			if value[idx] == quote {
				return value[1:idx], true
			}
		}
		return "", false
	}
	token := strings.TrimSuffix(strings.Fields(value)[0], ",")
	if !isStaticLiquidNameToken(token) {
		return token, false
	}
	return token, true
}

func isStaticLiquidNameToken(token string) bool {
	if token == "" || !staticLiquidNamePattern.MatchString(token) {
		return false
	}
	return !strings.Contains(token, ".") || strings.HasSuffix(token, ".liquid")
}

func normalizeLiquidDependencyName(name string) string {
	name = normalizePath(strings.TrimSpace(name))
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		return ""
	}
	if !strings.HasSuffix(name, ".liquid") {
		name += ".liquid"
	}
	return name
}

func sortResourceKeys(keys []resourceKey, byKey map[resourceKey]ResourceContent) {
	sort.SliceStable(keys, func(i, j int) bool {
		return resourceLess(byKey[keys[i]].Resource, byKey[keys[j]].Resource)
	})
}

func (k resourceKey) DisplayPath() string {
	return DisplayPath(k.kind, k.remoteName)
}
