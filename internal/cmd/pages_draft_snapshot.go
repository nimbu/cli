package cmd

import (
	"encoding/json"
	"path"
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
	if translations, ok := snapshotTranslations(snapshot["translations"]); ok {
		doc["translations"] = translations
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
		if snapshotItemDisabled(item) {
			// The live API serializes enabled items only; mirror that so a
			// draft document never shows editables pages get would hide.
			continue
		}
		converted, ok := convertSnapshotPageItem(item)
		if !ok {
			return nil, false
		}
		items[slug] = converted
	}
	return items, true
}

// snapshotItemStructuralKeys are snapshot keys that the converter rewrites into
// the page-document shape (or that are pure storage bookkeeping); everything
// else on a snapshot item is carried through verbatim.
var snapshotItemStructuralKeys = map[string]bool{
	"page_items":          true,
	"repeatables":         true,
	"translations":        true,
	"_id":                 true,
	"id":                  true,
	"source":              true,
	"source_content_type": true,
	"source_width":        true,
	"source_height":       true,
	"source_size":         true,
	"source_version":      true,
	"source_checksum":     true,
}

// convertSnapshotPageItem turns one Rails snapshot page_item (a raw Mongoid
// document) into the API page-document editable shape: every plain attribute is
// carried through, the source_* columns become a "file" object, reference_id(s)
// become a "reference" object, and the translations array becomes the
// locale-keyed map the live document uses.
func convertSnapshotPageItem(item map[string]any) (map[string]any, bool) {
	out := map[string]any{"type": item["type"]}
	if _, ok := item["content"]; !ok {
		out["content"] = nil
	}
	for key, value := range item {
		if snapshotItemStructuralKeys[key] {
			continue
		}
		out[key] = value
	}
	if id := snapshotID(item); id != "" {
		out["id"] = id
	}
	if id := snapshotIDValue(item["reference_id"]); id != "" {
		out["reference_id"] = id
	}
	if ids := snapshotIDList(item["reference_ids"]); len(ids) > 0 {
		out["reference_ids"] = ids
	}
	if file, ok := snapshotItemFile(item); ok {
		out["file"] = file
	}
	if reference, ok := snapshotItemReference(item); ok {
		out["reference"] = reference
	}
	if translations, ok := snapshotTranslations(item["translations"]); ok {
		out["translations"] = translations
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
		if snapshotItemDisabled(rep) {
			continue
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

// snapshotFileKeys maps the snapshot's source_* columns onto the keys the live
// API uses inside an editable's "file" object.
var snapshotFileKeys = map[string]string{
	"source_content_type": "content_type",
	"source_width":        "width",
	"source_height":       "height",
	"source_size":         "size",
	"source_version":      "version",
	"source_checksum":     "checksum",
}

// snapshotItemFile rebuilds a file object from the snapshot. The snapshot keeps
// the CarrierWave identifier in "source" plus the source_* metadata columns; it
// never carries the serialized "file" object, so without this a file editable
// renders as (empty) on a draft document.
func snapshotItemFile(item map[string]any) (map[string]any, bool) {
	if existing, ok := item["file"].(map[string]any); ok && len(existing) > 0 {
		return existing, true
	}
	source := stringAny(item["source"])
	if source == "" {
		return nil, false
	}
	file := map[string]any{"filename": path.Base(source)}
	if strings.Contains(source, "://") || strings.HasPrefix(source, "/") {
		file["url"] = source
	}
	for snapshotKey, fileKey := range snapshotFileKeys {
		if value, ok := item[snapshotKey]; ok && value != nil && value != "" {
			file[fileKey] = value
		}
	}
	return file, true
}

// snapshotItemReference rebuilds a reference object from the snapshot's
// reference_id / reference_ids / reference_type columns.
func snapshotItemReference(item map[string]any) (map[string]any, bool) {
	if existing, ok := item["reference"].(map[string]any); ok && len(existing) > 0 {
		return existing, true
	}
	out := map[string]any{}
	if id := snapshotIDValue(item["reference_id"]); id != "" {
		out["id"] = id
	}
	if ids := snapshotIDList(item["reference_ids"]); len(ids) > 0 {
		out["ids"] = ids
	}
	if kind := stringAny(item["reference_type"]); kind != "" {
		out["type"] = kind
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func snapshotIDList(v any) []any {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(list))
	for _, entry := range list {
		if id := snapshotIDValue(entry); id != "" {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// snapshotTranslations converts the snapshot's translations array
// ([{"locale":"en","content":"..."}]) into the locale-keyed map the live page
// document uses ({"en":{"content":"..."}}).
func snapshotTranslations(v any) (map[string]any, bool) {
	switch typed := v.(type) {
	case map[string]any:
		if len(typed) == 0 {
			return nil, false
		}
		return typed, true
	case []any:
		out := make(map[string]any, len(typed))
		for _, entry := range typed {
			translation, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			locale := stringAny(translation["locale"])
			if locale == "" {
				continue
			}
			fields := make(map[string]any, len(translation))
			for key, value := range translation {
				if key == "locale" || key == "_id" || key == "id" {
					continue
				}
				fields[key] = value
			}
			out[locale] = fields
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	default:
		return nil, false
	}
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
	if translations, ok := snapshotTranslations(rep["translations"]); ok {
		out["translations"] = translations
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

// snapshotItemDisabled reports whether a snapshot page item or repeatable is
// flagged disabled; the live API omits those, so the converter skips them too.
func snapshotItemDisabled(item map[string]any) bool {
	switch v := item["disabled"].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}
