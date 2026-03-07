package api

import (
	"context"
	"fmt"
	neturl "net/url"
	"strings"
)

// MenuDocument is the canonical nested menu contract used for get/update flows.
type MenuDocument map[string]any

// MenuDocumentStats summarizes a nested menu tree.
type MenuDocumentStats struct {
	ItemCount int
	MaxDepth  int
}

// GetMenuDocument fetches a menu document, falling back to the nested list contract when needed.
func GetMenuDocument(ctx context.Context, c *Client, identifier string, opts ...RequestOption) (MenuDocument, error) {
	identifier = strings.TrimSpace(identifier)
	path := "/menus/" + neturl.PathEscape(identifier)

	var doc MenuDocument
	err := c.Get(ctx, path, &doc, opts...)
	switch {
	case err == nil:
		if MenuDocumentHasItems(doc) {
			return doc, nil
		}
	case !IsNotFound(err):
		return nil, err
	}

	fallbackIdentifier := MenuDocumentSlug(doc)
	if fallbackIdentifier == "" {
		fallbackIdentifier = identifier
	}

	listOpts := append([]RequestOption{}, opts...)
	listOpts = append(listOpts, WithParam("nested", "1"), WithParam("slug", fallbackIdentifier))
	var menus []MenuDocument
	if err := c.Get(ctx, "/menus", &menus, listOpts...); err != nil {
		return nil, err
	}

	if selected, ok := SelectMenuDocument(menus, identifier); ok {
		return selected, nil
	}
	if doc != nil {
		return doc, nil
	}

	return nil, &Error{StatusCode: 404, Message: fmt.Sprintf("menu %q not found", identifier)}
}

// PatchMenuDocument updates a menu document with replace semantics.
func PatchMenuDocument(ctx context.Context, c *Client, slug string, doc MenuDocument, opts ...RequestOption) (MenuDocument, error) {
	var out MenuDocument
	path := "/menus/" + neturl.PathEscape(strings.TrimSpace(slug))
	opts = append(opts, WithParam("replace", "1"))
	if err := c.Patch(ctx, path, doc, &out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// MenuDocumentHasItems returns whether the raw menu response explicitly includes a nested items tree.
func MenuDocumentHasItems(doc MenuDocument) bool {
	if doc == nil {
		return false
	}
	_, ok := doc["items"]
	return ok
}

// MenuDocumentName returns the menu name when present.
func MenuDocumentName(doc MenuDocument) string {
	return stringValue(doc["name"])
}

// MenuDocumentHandle returns the menu handle when present.
func MenuDocumentHandle(doc MenuDocument) string {
	return stringValue(doc["handle"])
}

// MenuDocumentSlug returns the canonical slug when present.
func MenuDocumentSlug(doc MenuDocument) string {
	if slug := stringValue(doc["slug"]); slug != "" {
		return slug
	}
	return MenuDocumentHandle(doc)
}

// MenuStats counts items and depth in a nested menu tree.
func MenuStats(doc MenuDocument) MenuDocumentStats {
	stats := MenuDocumentStats{}
	items, ok := sliceValue(doc["items"])
	if !ok {
		return stats
	}
	stats.MaxDepth = menuDepth(items, 1, &stats.ItemCount)
	return stats
}

func menuDepth(items []any, depth int, count *int) int {
	maxDepth := 0
	for _, rawItem := range items {
		item, ok := mapValue(rawItem)
		if !ok {
			continue
		}
		*count++
		if depth > maxDepth {
			maxDepth = depth
		}
		children, ok := sliceValue(item["children"])
		if !ok || len(children) == 0 {
			continue
		}
		childDepth := menuDepth(children, depth+1, count)
		if childDepth > maxDepth {
			maxDepth = childDepth
		}
	}
	return maxDepth
}

// NormalizeMenuDocumentForWrite strips write-unsafe fields from a nested menu tree.
func NormalizeMenuDocumentForWrite(doc MenuDocument) {
	items, ok := sliceValue(doc["items"])
	if !ok {
		return
	}
	normalizeMenuItems(items)
}

func normalizeMenuItems(items []any) {
	for _, rawItem := range items {
		item, ok := mapValue(rawItem)
		if !ok {
			continue
		}
		delete(item, "target_page")
		children, ok := sliceValue(item["children"])
		if ok && len(children) > 0 {
			normalizeMenuItems(children)
		}
	}
}

// SelectMenuDocument matches a menu by slug or handle from a nested menu result set.
func SelectMenuDocument(menus []MenuDocument, identifier string) (MenuDocument, bool) {
	identifier = strings.TrimSpace(identifier)
	for _, menu := range menus {
		if MenuDocumentSlug(menu) == identifier || MenuDocumentHandle(menu) == identifier {
			return menu, true
		}
	}
	return nil, false
}
