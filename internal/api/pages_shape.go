package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
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

// pageEditableNoiseKeys are per-editable keys that carry no content.
var pageEditableNoiseKeys = []string{"created_at", "updated_at", "slug", "type"}

// pageCompactFileKeys are the file attributes kept by PageCompactDocument.
// Besides the display attributes it keeps every key the write path reads back
// (see ExpandPageAttachmentPathsWithOptions), so a compacted document still
// round-trips through pages update: dropping attachment_path would silently
// discard what --download-assets just wrote.
var pageCompactFileKeys = []string{
	"url",
	"filename",
	"width",
	"height",
	"attachment_path",
	"attachment_url",
	"attachment",
	"source",
	"__type",
}

// PageCompactDocument returns a display-only copy of the page document with
// per-editable bookkeeping removed: editable created_at/updated_at/slug/type,
// repeatable created_at/updated_at, translation entries that merely repeat the
// default-locale content, and file objects reduced to url/filename/width/height.
// Repeatable ids, positions and slugs are kept.
func PageCompactDocument(doc PageDocument) (PageDocument, error) {
	copied, err := deepCopyPageDocument(doc)
	if err != nil {
		return nil, err
	}
	if og, ok := mapValue(copied["og_image"]); ok {
		copied["og_image"] = compactPageFile(og)
	}
	compactPageTranslations(copied)
	if items, ok := mapValue(copied["items"]); ok {
		compactPageEditables(items)
	}
	return copied, nil
}

func compactPageEditables(items map[string]any) {
	for _, raw := range items {
		editable, ok := mapValue(raw)
		if !ok {
			continue
		}
		for _, key := range pageEditableNoiseKeys {
			delete(editable, key)
		}
		if file, ok := mapValue(editable["file"]); ok {
			editable["file"] = compactPageFile(file)
		}
		if translations, ok := mapValue(editable["translations"]); ok {
			for _, rawTranslation := range translations {
				translation, ok := mapValue(rawTranslation)
				if !ok {
					continue
				}
				for _, key := range pageEditableNoiseKeys {
					delete(translation, key)
				}
				if file, ok := mapValue(translation["file"]); ok {
					translation["file"] = compactPageFile(file)
				}
			}
		}
		compactPageTranslations(editable)

		repeatables, ok := sliceValue(editable["repeatables"])
		if !ok {
			continue
		}
		for _, rawRepeatable := range repeatables {
			repeatable, ok := mapValue(rawRepeatable)
			if !ok {
				continue
			}
			delete(repeatable, "created_at")
			delete(repeatable, "updated_at")
			compactPageTranslations(repeatable)
			if childItems, ok := mapValue(repeatable["items"]); ok {
				compactPageEditables(childItems)
			}
		}
	}
}

// compactPageTranslations drops translation entries that are fully redundant
// with the values already present on the owning object (the default locale).
func compactPageTranslations(owner map[string]any) {
	translations, ok := mapValue(owner["translations"])
	if !ok {
		return
	}
	for locale, rawTranslation := range translations {
		translation, ok := mapValue(rawTranslation)
		if !ok {
			continue
		}
		if translationMatchesOwner(owner, translation) {
			delete(translations, locale)
		}
	}
	if len(translations) == 0 {
		delete(owner, "translations")
	}
}

func translationMatchesOwner(owner, translation map[string]any) bool {
	for key, value := range translation {
		current, ok := owner[key]
		if !ok || !reflect.DeepEqual(current, value) {
			return false
		}
	}
	return true
}

func compactPageFile(file map[string]any) map[string]any {
	compact := map[string]any{}
	for _, key := range pageCompactFileKeys {
		if value, ok := file[key]; ok && value != nil {
			compact[key] = value
		}
	}
	return compact
}

// PageProjectFields keeps only the requested top-level keys of a page
// document. Unknown field names are reported as an error.
func PageProjectFields(doc PageDocument, fields []string) (PageDocument, error) {
	if len(fields) == 0 {
		return doc, nil
	}
	projected := PageDocument{}
	var unknown []string
	for _, field := range fields {
		value, ok := doc[field]
		if !ok {
			unknown = append(unknown, field)
			continue
		}
		projected[field] = value
	}
	if len(unknown) > 0 {
		available := make([]string, 0, len(doc))
		for key := range doc {
			available = append(available, key)
		}
		sort.Strings(available)
		return nil, fmt.Errorf("unknown field(s) %s; available: %s",
			strings.Join(unknown, ", "), strings.Join(available, ", "))
	}
	return projected, nil
}

func deepCopyPageDocument(doc PageDocument) (PageDocument, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("encode page document: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var copied PageDocument
	if err := decoder.Decode(&copied); err != nil {
		return nil, fmt.Errorf("decode page document: %w", err)
	}
	return copied, nil
}
