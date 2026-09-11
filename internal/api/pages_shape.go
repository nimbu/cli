package api

import (
	"strings"

	"github.com/nimbu/cli/internal/pagepath"
)

// PageShape returns a skeleton of the page's editables: editable name -> type,
// and for canvases the list of repeatables with their slug and nested skeleton.
func PageShape(doc PageDocument) any {
	return PageShapeWithSchema(doc, nil)
}

// PageShapeWithSchema is PageShape plus select options when a schema is available.
func PageShapeWithSchema(doc PageDocument, schema *pagepath.Schema) any {
	items, ok := mapValue(doc["items"])
	if !ok {
		return map[string]any{}
	}
	return pageShapeItems(items, schema, "", "")
}

func pageShapeItems(items map[string]any, schema *pagepath.Schema, canvas, slug string) map[string]any {
	shape := map[string]any{}
	for name, rawEditable := range items {
		editable, ok := mapValue(rawEditable)
		if !ok {
			continue
		}
		entry := map[string]any{
			"type": stringValue(editable["type"]),
		}
		if opts := schemaOptionLabels(schema, canvas, slug, name); len(opts) > 0 {
			entry["options"] = opts
		}
		if repeatables, ok := sliceValue(editable["repeatables"]); ok {
			ordered := orderedShapeRepeatables(repeatables)
			reps := make([]any, 0, len(ordered))
			for index, repeatable := range ordered {
				repSlug := stringValue(repeatable["slug"])
				rep := map[string]any{
					"id":       stringValue(repeatable["id"]),
					"position": index,
					"slug":     repSlug,
				}
				if childItems, ok := mapValue(repeatable["items"]); ok {
					rep["items"] = pageShapeItems(childItems, schema, name, repSlug)
				} else {
					rep["items"] = map[string]any{}
				}
				reps = append(reps, rep)
			}
			entry["repeatables"] = reps
		}
		shape[name] = entry
	}
	return shape
}

func schemaOptionLabels(schema *pagepath.Schema, canvas, slug, field string) []string {
	opts := schema.OptionsFor(canvas, slug, field)
	if len(opts) == 0 {
		return nil
	}
	labels := make([]string, 0, len(opts))
	for _, opt := range opts {
		label := strings.TrimSpace(opt.Label)
		if label == "" {
			label = strings.TrimSpace(opt.Value)
		}
		if label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}

func orderedShapeRepeatables(repeatables []any) []map[string]any {
	type pair struct {
		item map[string]any
		pos  float64
	}
	pairs := make([]pair, 0, len(repeatables))
	for _, raw := range repeatables {
		item, ok := mapValue(raw)
		if !ok {
			continue
		}
		pairs = append(pairs, pair{item: item, pos: shapePosition(item["position"])})
	}
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].pos < pairs[i].pos {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
	out := make([]map[string]any, len(pairs))
	for i, p := range pairs {
		out[i] = p.item
	}
	return out
}

func shapePosition(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}
