package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

// schemaTargetDrift inventories live targets absent from local schema files.
func schemaTargetDrift(ctx context.Context, client *api.Client, docs []schemaDocument) ([]schemaPlan, error) {
	declared := make(map[string]bool, len(docs))
	kinds := make(map[string]bool)
	for _, doc := range docs {
		declared[doc.Target] = true
		kind, _, collection := strings.Cut(doc.Target, ":")
		if collection {
			kinds[kind] = true
		}
	}
	drift := make([]schemaPlan, 0)
	seen := make(map[string]bool)
	for _, resource := range []struct{ path, kind string }{
		{"/channels", "channel"},
		{"/blogs", "blog"},
		{"/products/checkout_profiles", "checkout_profile"},
	} {
		if !kinds[resource.kind] {
			continue
		}
		targets, err := api.List[map[string]any](ctx, client, resource.path)
		if err != nil {
			return nil, fmt.Errorf("inventory %s schemas: %w", resource.kind, err)
		}
		for _, item := range targets {
			slug, _ := item["slug"].(string)
			if slug == "" {
				slug, _ = item["handle"].(string)
			}
			if slug == "" {
				return nil, fmt.Errorf("inventory %s schemas: target omitted slug", resource.kind)
			}
			target := resource.kind + ":" + slug
			if declared[target] || seen[target] {
				continue
			}
			seen[target] = true
			drift = append(drift, schemaPlan{Target: target, Exists: true, Drift: []map[string]any{{"target": target, "note": "exists on target, absent from local schema files"}}})
		}
	}
	sort.Slice(drift, func(i, j int) bool { return drift[i].Target < drift[j].Target })
	return drift, nil
}
