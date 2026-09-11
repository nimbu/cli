package api

import (
	"context"
	"fmt"
	neturl "net/url"
	"strings"
)

// PageDocument is the canonical page contract used for get/update flows.
type PageDocument map[string]any

// PageDocumentStats summarizes nested editables inside a page document.
type PageDocumentStats struct {
	AttachmentCount int
	EditableCount   int
}

// GetPageDocument fetches the full page document by fullpath.
func GetPageDocument(ctx context.Context, c *Client, fullpath string, opts ...RequestOption) (PageDocument, error) {
	var doc PageDocument
	path := "/pages/" + neturl.PathEscape(NormalizePageFullpath(fullpath))
	if err := c.Get(ctx, path, &doc, opts...); err != nil {
		return nil, err
	}
	return doc, nil
}

// PatchPageDocument updates a page document. The caller controls merge vs.
// replace semantics via opts (see WithReplace); by default the API merges.
func PatchPageDocument(ctx context.Context, c *Client, fullpath string, doc PageDocument, opts ...RequestOption) (PageDocument, error) {
	var out PageDocument
	path := "/pages/" + neturl.PathEscape(NormalizePageFullpath(fullpath))
	if err := c.Patch(ctx, path, doc, &out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// pageReadOnlyKeys are server-managed top-level keys that must never be written back.
var pageReadOnlyKeys = []string{
	"id",
	"created_at",
	"updated_at",
	"creator_id",
	"updater_id",
	"parent_path",
}

// NormalizePageDocumentForWrite removes server-managed top-level keys so the
// document can be safely sent on create/update requests.
func NormalizePageDocumentForWrite(doc PageDocument) {
	for _, key := range pageReadOnlyKeys {
		delete(doc, key)
	}
}

// PageCanvasRepeatableCounts returns, for each canvas editable path, the number
// of repeatables it contains. Nested paths use dot notation and aggregate
// repeatables across repeated instances of the same editable path.
func PageCanvasRepeatableCounts(doc PageDocument) map[string]int {
	counts := map[string]int{}
	items, ok := mapValue(doc["items"])
	if !ok {
		return counts
	}
	pageCanvasRepeatableCounts(items, "", counts)
	return counts
}

func pageCanvasRepeatableCounts(items map[string]any, prefix string, counts map[string]int) {
	for name, rawEditable := range items {
		editable, ok := mapValue(rawEditable)
		if !ok {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		repeatables, ok := sliceValue(editable["repeatables"])
		if !ok {
			continue
		}
		counts[path] += len(repeatables)
		for _, rawRepeatable := range repeatables {
			repeatable, ok := mapValue(rawRepeatable)
			if !ok {
				continue
			}
			childItems, ok := mapValue(repeatable["items"])
			if !ok {
				continue
			}
			pageCanvasRepeatableCounts(childItems, path, counts)
		}
	}
}

// PageCanvasRepeatableInstanceCounts returns repeatable counts for each canvas
// instance. Nested paths include repeatable indexes, e.g. blocks[0].gallery.
func PageCanvasRepeatableInstanceCounts(doc PageDocument) map[string]int {
	counts := map[string]int{}
	items, ok := mapValue(doc["items"])
	if !ok {
		return counts
	}
	pageCanvasRepeatableInstanceCounts(items, "", counts)
	return counts
}

func pageCanvasRepeatableInstanceCounts(items map[string]any, prefix string, counts map[string]int) {
	for name, rawEditable := range items {
		editable, ok := mapValue(rawEditable)
		if !ok {
			continue
		}
		currentPath := name
		if prefix != "" {
			currentPath = prefix + "." + name
		}
		repeatables, ok := sliceValue(editable["repeatables"])
		if !ok {
			continue
		}
		counts[currentPath] = len(repeatables)
		for index, rawRepeatable := range repeatables {
			repeatable, ok := mapValue(rawRepeatable)
			if !ok {
				continue
			}
			childItems, ok := mapValue(repeatable["items"])
			if !ok {
				continue
			}
			pageCanvasRepeatableInstanceCounts(childItems, fmt.Sprintf("%s[%d]", currentPath, index), counts)
		}
	}
}

// NormalizePageFullpath strips leading slashes and whitespace from a page fullpath.
func NormalizePageFullpath(fullpath string) string {
	fullpath = strings.TrimSpace(fullpath)
	return strings.TrimLeft(fullpath, "/")
}

// PageDocumentFullpath returns the canonical page fullpath when present.
func PageDocumentFullpath(doc PageDocument) string {
	if value := stringValue(doc["fullpath"]); value != "" {
		return NormalizePageFullpath(value)
	}
	return NormalizePageFullpath(stringValue(doc["slug"]))
}

// PageDocumentParentPath returns the parent path for a page when present.
func PageDocumentParentPath(doc PageDocument) string {
	if value := stringValue(doc["parent_path"]); value != "" {
		return NormalizePageFullpath(value)
	}
	if value := stringValue(doc["parent"]); value != "" {
		return NormalizePageFullpath(value)
	}
	return ""
}

// PageDocumentTitle returns the page title when present.
func PageDocumentTitle(doc PageDocument) string {
	return stringValue(doc["title"])
}

// PageDocumentTemplate returns the page template when present.
func PageDocumentTemplate(doc PageDocument) string {
	return stringValue(doc["template"])
}

// PageDocumentLocale returns the page locale when present.
func PageDocumentLocale(doc PageDocument) string {
	return stringValue(doc["locale"])
}

// PageDocumentPublished returns whether the page is published.
func PageDocumentPublished(doc PageDocument) bool {
	return boolValue(doc["published"])
}

// PageStats traverses the page document and counts editables and attachment-bearing file objects.
func PageStats(doc PageDocument) PageDocumentStats {
	stats := PageDocumentStats{}
	_ = WalkPageEditables(doc, func(_ string, editable map[string]any) error {
		stats.EditableCount++
		file := PageEditableFile(editable)
		if file != nil && pageFileHasAttachment(file) {
			stats.AttachmentCount++
		}
		return nil
	})
	return stats
}

// WalkPageEditables traverses all editables in a page document, including nested repeatables.
func WalkPageEditables(doc PageDocument, fn func(name string, editable map[string]any) error) error {
	items, ok := mapValue(doc["items"])
	if !ok {
		return nil
	}
	return walkPageEditableItems(items, fn)
}

func walkPageEditableItems(items map[string]any, fn func(name string, editable map[string]any) error) error {
	for name, rawEditable := range items {
		editable, ok := mapValue(rawEditable)
		if !ok {
			continue
		}
		if err := fn(name, editable); err != nil {
			return err
		}

		repeatables, ok := sliceValue(editable["repeatables"])
		if !ok {
			continue
		}
		for _, rawRepeatable := range repeatables {
			repeatable, ok := mapValue(rawRepeatable)
			if !ok {
				continue
			}
			childItems, ok := mapValue(repeatable["items"])
			if !ok {
				continue
			}
			if err := walkPageEditableItems(childItems, fn); err != nil {
				return err
			}
		}
	}
	return nil
}

// PageEditableFile returns the file map from an editable, or nil if absent.
func PageEditableFile(editable map[string]any) map[string]any {
	file, ok := mapValue(editable["file"])
	if !ok {
		return nil
	}
	return file
}

func mapValue(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func sliceValue(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func stringValue(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func boolValue(v any) bool {
	b, ok := v.(bool)
	return ok && b
}
