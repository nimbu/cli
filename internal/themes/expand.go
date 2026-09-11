package themes

import (
	"fmt"
	"strings"
)

func expandUploadsWithLocalDependencies(uploads, allLocal []Resource) ([]Resource, []AddedDependency, []string, error) {
	localItems, err := resourceContents(allLocal)
	if err != nil {
		return nil, nil, nil, err
	}
	localByKey := make(map[resourceKey]ResourceContent, len(localItems))
	for _, item := range localItems {
		localByKey[keyFor(item.Resource)] = item
	}
	selected := make([]ResourceContent, 0, len(uploads))
	for _, resource := range uploads {
		if item, ok := localByKey[keyFor(resource)]; ok {
			selected = append(selected, item)
		}
	}
	expandedItems, added, warnings := ExpandTransferSetWithLiquidDependencies(selected, localItems)
	expanded := make([]Resource, len(expandedItems))
	for i, item := range expandedItems {
		expanded[i] = item.Resource
	}
	return expanded, added, warnings, nil
}

func resourceContents(resources []Resource) ([]ResourceContent, error) {
	items := make([]ResourceContent, 0, len(resources))
	for _, resource := range resources {
		item := ResourceContent{Resource: resource}
		if resource.Kind != KindAsset {
			content, err := readFile(resource.AbsPath)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", resource.DisplayPath, err)
			}
			item.Content = content
		}
		items = append(items, item)
	}
	return items, nil
}

func dropTransferSetWarnings(warnings []string) []string {
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if strings.HasPrefix(warning, "dependency not in transfer set:") {
			continue
		}
		out = append(out, warning)
	}
	return out
}
