package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

func schemaCycleTargets(docs []schemaDocument) map[string]bool {
	graph := map[string][]string{}
	for _, doc := range docs {
		if !strings.HasPrefix(doc.Endpoint, "/channels/") {
			continue
		}
		slug, _ := doc.Body["slug"].(string)
		graph[slug] = nil
		for _, raw := range doc.Body["fields"].([]any) {
			field, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if ref := schemaChannelReference(field); ref != "" {
				graph[slug] = append(graph[slug], ref)
			}
		}
	}
	cyclic := map[string]bool{}
	for origin := range graph {
		seen := map[string]bool{}
		var reaches func(string) bool
		reaches = func(at string) bool {
			for _, next := range graph[at] {
				if next == origin {
					return true
				}
				if !seen[next] {
					seen[next] = true
					if reaches(next) {
						return true
					}
				}
			}
			return false
		}
		if reaches(origin) {
			cyclic[origin] = true
		}
	}
	return cyclic
}

// Placeholders are created only by an explicitly confirmed schema apply, never by plan.
func prepareSchemaCycles(ctx context.Context, client *api.Client, docs []schemaDocument, plans []schemaPlan) (map[int]string, error) {
	created := map[int]string{}
	cyclic := schemaCycleTargets(docs)
	for i, doc := range docs {
		slug, _ := doc.Body["slug"].(string)
		if !strings.HasPrefix(doc.Endpoint, "/channels/") {
			continue
		}
		if !cyclic[slug] || plans[i].Exists {
			continue
		}
		placeholder := schemaDocument{Target: doc.Target, Endpoint: doc.Endpoint, Body: map[string]any{"slug": slug, "name": slug, "fields": []any{map[string]any{"name": "nimbu_schema_placeholder", "label": "Temporary schema placeholder", "type": "string"}}}}
		plan, err := fetchSchemaPlan(ctx, client, placeholder, false)
		if err != nil {
			return created, err
		}
		if plan.Exists {
			return created, fmt.Errorf("%s appeared while preparing circular references; inspect and re-plan", doc.Target)
		}
		if schemaPlanExit([]schemaPlan{plan}) == 1 {
			return created, fmt.Errorf("cannot prepare circular reference placeholder for %s", doc.Target)
		}
		body := schemaRequestBody(placeholder, false)
		body["fingerprint"] = plan.Fingerprint
		var result map[string]any
		if err = client.Post(ctx, doc.Endpoint+"/apply", body, &result); err != nil {
			return created, fmt.Errorf("create circular placeholder %s: %w", doc.Target, err)
		}
		if result["applied"] != true {
			return created, fmt.Errorf("create circular placeholder %s: missing success confirmation", doc.Target)
		}
		fingerprint, _ := result["fingerprint"].(string)
		if fingerprint == "" {
			return created, fmt.Errorf("create circular placeholder %s: missing resulting fingerprint; target may have been created", doc.Target)
		}
		created[i] = fingerprint
	}
	return created, nil
}

func schemaChannelReference(field map[string]any) string {
	reference, _ := field["reference"].(string)
	switch reference {
	case "products", "customers", "orders", "pages", "articles":
		return ""
	default:
		return reference
	}
}
