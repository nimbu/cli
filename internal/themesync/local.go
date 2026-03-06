package themesync

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// CollectAll returns every managed local resource discovered under configured roots.
func CollectAll(cfg Config) ([]Resource, error) {
	ignoreMatchers, err := compileMatchers(cfg.Ignore)
	if err != nil {
		return nil, fmt.Errorf("compile ignore patterns: %w", err)
	}

	resources := map[resourceKey]Resource{}
	for _, root := range orderedRoots(cfg.Roots) {
		if err := walkRoot(cfg, root, ignoreMatchers, resources); err != nil {
			return nil, err
		}
	}

	return sortResources(resources), nil
}

// CollectGenerated returns generated resources that currently exist on disk.
func CollectGenerated(cfg Config) ([]Resource, error) {
	ignoreMatchers, err := compileMatchers(cfg.Ignore)
	if err != nil {
		return nil, fmt.Errorf("compile ignore patterns: %w", err)
	}
	generatedMatchers, err := compileMatchers(cfg.Generated)
	if err != nil {
		return nil, fmt.Errorf("compile generated patterns: %w", err)
	}

	resources := map[resourceKey]Resource{}
	err = filepath.WalkDir(cfg.ProjectRoot, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == cfg.ProjectRoot {
			return nil
		}

		rel, err := projectRelativePath(cfg.ProjectRoot, current)
		if err != nil {
			return nil
		}
		rel = normalizePath(rel)
		if isHiddenPath(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if matchesAny(ignoreMatchers, rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if !matchesAny(generatedMatchers, rel) {
			return nil
		}

		resource, ok, err := ClassifyProjectPath(cfg, rel)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := addResource(resources, resource); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return sortResources(resources), nil
}

// ClassifyProjectPath maps a project-relative path onto a configured resource kind.
func ClassifyProjectPath(cfg Config, relPath string) (Resource, bool, error) {
	normalized := normalizePath(relPath)
	for _, root := range orderedRoots(cfg.Roots) {
		if !pathMatchesRoot(normalized, root.LocalPath) {
			continue
		}

		remoteName := normalized
		if root.LocalPath != "" {
			remoteName = strings.TrimPrefix(normalized, root.LocalPath)
			remoteName = strings.TrimPrefix(remoteName, "/")
		}
		if root.Kind == KindAsset {
			remoteName = path.Join(root.RemoteBase, remoteName)
		}
		remoteName = normalizePath(remoteName)
		if remoteName == "." || remoteName == "" {
			if root.Kind == KindAsset && root.RemoteBase != "" {
				remoteName = root.RemoteBase
			} else {
				remoteName = path.Base(normalized)
			}
		}

		absPath := filepath.Join(cfg.ProjectRoot, filepath.FromSlash(normalized))
		resource := Resource{
			AbsPath:     absPath,
			DisplayPath: DisplayPath(root.Kind, remoteName),
			Kind:        root.Kind,
			LocalPath:   normalized,
			RemoteName:  remoteName,
		}
		return resource, true, nil
	}
	return Resource{}, false, nil
}

func walkRoot(cfg Config, root RootSpec, ignoreMatchers []globMatcher, resources map[resourceKey]Resource) error {
	info, err := os.Stat(root.AbsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat root %s: %w", root.LocalPath, err)
	}

	if !info.IsDir() {
		rel, err := projectRelativePath(cfg.ProjectRoot, root.AbsPath)
		if err != nil {
			return err
		}
		rel = normalizePath(rel)
		if isHiddenPath(rel) || matchesAny(ignoreMatchers, rel) {
			return nil
		}
		resource, ok, err := ClassifyProjectPath(cfg, rel)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		return addResource(resources, resource)
	}

	return filepath.WalkDir(root.AbsPath, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root.AbsPath {
			return nil
		}

		rel, err := projectRelativePath(cfg.ProjectRoot, current)
		if err != nil {
			return nil
		}
		rel = normalizePath(rel)
		if isHiddenPath(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if matchesAny(ignoreMatchers, rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}

		resource, ok, err := ClassifyProjectPath(cfg, rel)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		return addResource(resources, resource)
	})
}

func addResource(resources map[resourceKey]Resource, resource Resource) error {
	key := keyFor(resource)
	if existing, ok := resources[key]; ok && existing.LocalPath != resource.LocalPath {
		return fmt.Errorf("duplicate remote theme resource %s from %s and %s", resource.DisplayPath, existing.LocalPath, resource.LocalPath)
	}
	resources[key] = resource
	return nil
}

func orderedRoots(roots []RootSpec) []RootSpec {
	ordered := append([]RootSpec{}, roots...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := normalizePath(ordered[i].LocalPath)
		right := normalizePath(ordered[j].LocalPath)
		if len(left) == len(right) {
			return left < right
		}
		return len(left) > len(right)
	})
	return ordered
}

func pathMatchesRoot(relPath, root string) bool {
	root = normalizePath(root)
	if root == "" || root == "." {
		return true
	}
	return relPath == root || strings.HasPrefix(relPath, root+"/")
}

func isHiddenPath(relPath string) bool {
	for _, segment := range strings.Split(normalizePath(relPath), "/") {
		if strings.HasPrefix(segment, ".") {
			return true
		}
	}
	return false
}

func sortResources(resources map[resourceKey]Resource) []Resource {
	items := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		items = append(items, resource)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].DisplayPath == items[j].DisplayPath {
			return items[i].Kind < items[j].Kind
		}
		return items[i].DisplayPath < items[j].DisplayPath
	})
	return items
}
