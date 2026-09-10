package cmd

import (
	"encoding/json"
	"strconv"
	"strings"
)

var draftSnapshotPageFields = []string{
	"title",
	"slug",
	"seo_title",
	"seo_description",
	"seo_keywords",
	"published",
	"og_image",
}

// draftSnapshotToDocument turns a Rails Pages::SnapshotBuilder content hash
// (page_items + nested repeatables/page_items) into the API page document
// items-map shape used by path resolution.
func draftSnapshotToDocument(snapshot map[string]any) (map[string]any, bool) {
	if snapshot == nil {
		return nil, false
	}
	rawItems, ok := snapshot["page_items"].([]any)
	if !ok {
		return nil, false
	}
	items, ok := convertSnapshotPageItems(rawItems)
	if !ok {
		return nil, false
	}
	doc := map[string]any{"items": items}
	for _, key := range draftSnapshotPageFields {
		if value, exists := snapshot[key]; exists {
			doc[key] = value
		}
	}
	return doc, true
}

func convertSnapshotPageItems(raw []any) (map[string]any, bool) {
	items := make(map[string]any, len(raw))
	for _, entry := range raw {
		item, ok := entry.(map[string]any)
		if !ok {
			return nil, false
		}
		slug := stringAny(item["slug"])
		if slug == "" {
			return nil, false
		}
		converted, ok := convertSnapshotPageItem(item)
		if !ok {
			return nil, false
		}
		items[slug] = converted
	}
	return items, true
}

func convertSnapshotPageItem(item map[string]any) (map[string]any, bool) {
	out := map[string]any{
		"type":    item["type"],
		"content": item["content"],
	}
	rawReps, hasReps := item["repeatables"]
	if !hasReps {
		return out, true
	}
	list, ok := rawReps.([]any)
	if !ok {
		return nil, false
	}
	reps := make([]any, 0, len(list))
	for _, entry := range list {
		rep, ok := entry.(map[string]any)
		if !ok {
			return nil, false
		}
		converted, ok := convertSnapshotRepeatable(rep)
		if !ok {
			return nil, false
		}
		reps = append(reps, converted)
	}
	out["repeatables"] = reps
	return out, true
}

func convertSnapshotRepeatable(rep map[string]any) (map[string]any, bool) {
	id := snapshotID(rep)
	if id == "" {
		return nil, false
	}
	items, ok := convertSnapshotPageItems(asAnySlice(rep["page_items"]))
	if !ok {
		return nil, false
	}
	out := map[string]any{
		"id":    id,
		"slug":  stringAny(rep["slug"]),
		"items": items,
	}
	if pos, ok := snapshotPosition(rep["position"]); ok {
		out["position"] = pos
	}
	return out, true
}

func asAnySlice(v any) []any {
	list, _ := v.([]any)
	return list
}

func snapshotID(m map[string]any) string {
	for _, key := range []string{"id", "_id"} {
		if id := snapshotIDValue(m[key]); id != "" {
			return id
		}
	}
	return ""
}

func snapshotIDValue(v any) string {
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if oid := stringAny(typed["$oid"]); oid != "" {
			return oid
		}
	}
	return ""
}

func snapshotPosition(v any) (int, bool) {
	switch typed := v.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		n, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}
