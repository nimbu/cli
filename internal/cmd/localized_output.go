package cmd

import (
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

func localizedDocumentMap[T any](document api.Document[T], locale string) (map[string]any, error) {
	projected, err := output.ProjectLocale(document, locale)
	if err != nil {
		return nil, err
	}
	row, ok := projected.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("localized document is %T, want object", projected)
	}
	return row, nil
}

func applyBlogDisplayHandle(blog map[string]any) {
	if handle, _ := blog["handle"].(string); strings.TrimSpace(handle) != "" {
		return
	}
	if slug, _ := blog["slug"].(string); strings.TrimSpace(slug) != "" {
		blog["handle"] = slug
		return
	}
	if id, _ := blog["id"].(string); strings.TrimSpace(id) != "" {
		blog["handle"] = id
		return
	}
	blog["handle"] = "-"
}

func localizedDocumentMaps[T any](documents []api.Document[T], locale string) ([]map[string]any, error) {
	if len(documents) == 0 {
		return []map[string]any{}, nil
	}
	projected, err := output.ProjectLocale(documents, locale)
	if err != nil {
		return nil, err
	}
	items, ok := projected.([]any)
	if !ok {
		return nil, fmt.Errorf("localized documents are %T, want array", projected)
	}
	rows := make([]map[string]any, len(items))
	for i, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("localized document %d is %T, want object", i, item)
		}
		rows[i] = row
	}
	return rows, nil
}
