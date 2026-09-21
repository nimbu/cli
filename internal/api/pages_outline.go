package api

import (
	"fmt"
	"sort"

	"github.com/nimbu/cli/internal/pagepath"
)

// Outline entry types that are not editable types.
const (
	PageOutlineTypePage       = "page"
	PageOutlineTypeCanvas     = "canvas"
	PageOutlineTypeRepeatable = "repeatable"
)

// pageOutlineFields are the page-level fields reported before the editables.
var pageOutlineFields = []string{
	"title",
	"slug",
	"seo_title",
	"seo_description",
	"seo_keywords",
	"published",
	"og_image",
	"security_mechanism",
	"template",
}

// outlineMaxCanvasLevels mirrors pagepath.Resolve, which refuses to descend
// past two canvas levels. Deeper entries are still listed, but they get no
// human path because no human path could address them.
const outlineMaxCanvasLevels = 2

// PageOutlineEntry is one row of a page outline: a page field, a canvas, a
// repeatable instance, or an editable inside one. Path is empty when no human
// path can address the entry; RawPath always can.
type PageOutlineEntry struct {
	Path    string `json:"path"`
	RawPath string `json:"raw_path"`
	ID      string `json:"id,omitempty"`
	Slug    string `json:"slug,omitempty"`
	Type    string `json:"type"`
	Content any    `json:"content,omitempty"`

	// Label is the short display name for text output ("Blokken[0]"), Depth
	// the indent level, Repeatables the number of repeatables on a canvas
	// entry. None of them are serialized.
	Label       string `json:"-"`
	Depth       int    `json:"-"`
	Repeatables int    `json:"-"`
}

// PageOutline flattens a page document into an ordered, readable listing:
// page-level fields first, then every editable in canvas/repeatable order.
// Paths are valid `pages set --path` inputs; RawPath is the API path.
func PageOutline(doc PageDocument) []PageOutlineEntry {
	entries := make([]PageOutlineEntry, 0, 32)
	for _, field := range pageOutlineFields {
		value, ok := doc[field]
		if !ok || value == nil {
			continue
		}
		entries = append(entries, PageOutlineEntry{
			Path:    field,
			RawPath: "/" + field,
			Slug:    field,
			Type:    PageOutlineTypePage,
			Content: value,
			Label:   field,
		})
	}

	items, ok := mapValue(doc["items"])
	if !ok {
		return entries
	}
	return appendPageOutlineItems(entries, items, nil, "", 0, false)
}

// outlineHumanPath renders segments as a human path, or returns "" when the
// path cannot be addressed by hand: the name collides with a page field
// (unaddressable), or it needs more canvas levels than pagepath.Resolve walks.
func outlineHumanPath(segments []pagepath.Segment, unaddressable bool) string {
	if unaddressable {
		return ""
	}
	selectors := 0
	for _, segment := range segments {
		if segment.Selector != nil {
			selectors++
		}
	}
	if selectors > outlineMaxCanvasLevels {
		return ""
	}
	return pagepath.Path{Segments: segments}.String()
}

func appendPageOutlineItems(entries []PageOutlineEntry, items map[string]any, prefix []pagepath.Segment, rawPrefix string, depth int, unaddressable bool) []PageOutlineEntry {
	for _, name := range sortedItemNames(items) {
		editable, ok := mapValue(items[name])
		if !ok {
			continue
		}
		segments := append(append([]pagepath.Segment{}, prefix...), pagepath.Segment{Name: name})
		raw := rawPrefix + "/items/" + pagepath.EscapeName(name)
		// A top-level editable named like a page field shadows itself: the
		// human path would parse back as the page field, so it and everything
		// under it can only be addressed raw.
		blocked := unaddressable || (len(prefix) == 0 && pagepath.IsPageField(name))

		repeatables, isCanvas := sliceValue(editable["repeatables"])
		if !isCanvas && stringValue(editable["type"]) != PageOutlineTypeCanvas {
			entries = append(entries, PageOutlineEntry{
				Path:    outlineHumanPath(segments, blocked),
				RawPath: raw,
				Slug:    name,
				Type:    stringValue(editable["type"]),
				Content: PageOutlineContent(editable),
				Label:   name,
				Depth:   depth,
			})
			continue
		}

		ordered := orderedShapeRepeatables(repeatables)
		entries = append(entries, PageOutlineEntry{
			Path:        outlineHumanPath(segments, blocked),
			RawPath:     raw,
			Slug:        name,
			Type:        PageOutlineTypeCanvas,
			Label:       name,
			Depth:       depth,
			Repeatables: len(ordered),
		})

		// A top-level canvas needs no indent level of its own: its repeatable
		// lines already carry the canvas name.
		repDepth := depth + 1
		if depth == 0 {
			repDepth = 0
		}
		for index, repeatable := range ordered {
			id := stringValue(repeatable["id"])
			repSegments := append(append([]pagepath.Segment{}, prefix...), pagepath.Segment{
				Name:     name,
				Selector: &pagepath.Selector{Kind: pagepath.SelIndex, Index: index},
			})
			repRaw := raw + "/repeatables/" + id
			entries = append(entries, PageOutlineEntry{
				Path:    outlineHumanPath(repSegments, blocked),
				RawPath: repRaw,
				ID:      id,
				Slug:    stringValue(repeatable["slug"]),
				Type:    PageOutlineTypeRepeatable,
				Label:   fmt.Sprintf("%s[%d]", name, index),
				Depth:   repDepth,
			})
			childItems, ok := mapValue(repeatable["items"])
			if !ok {
				continue
			}
			entries = appendPageOutlineItems(entries, childItems, repSegments, repRaw, repDepth+1, blocked)
		}
	}
	return entries
}

// PageOutlineContent returns the readable value of an editable: its content,
// else its file object, else its reference object, else nil.
func PageOutlineContent(editable map[string]any) any {
	if value, ok := editable["content"]; ok && value != nil {
		if text, isString := value.(string); isString && text == "" {
			return nil
		}
		return value
	}
	if file, ok := mapValue(editable["file"]); ok && len(file) > 0 {
		return file
	}
	if reference, ok := mapValue(editable["reference"]); ok && len(reference) > 0 {
		return reference
	}
	return nil
}

func sortedItemNames(items map[string]any) []string {
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
