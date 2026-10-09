package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	neturl "net/url"
	"slices"
	"strings"
)

// MenuDocument is the canonical nested menu contract used for get/update flows.
type MenuDocument map[string]any

// MenuDocumentStats summarizes a nested menu tree.
type MenuDocumentStats struct {
	HasItems  bool
	ItemCount int
	MaxDepth  int
	Shape     string
}

// GetMenuDocument fetches a menu document, falling back to the nested list contract when needed.
func GetMenuDocument(ctx context.Context, c *Client, identifier string, opts ...RequestOption) (MenuDocument, error) {
	identifier = strings.TrimSpace(identifier)
	path := "/menus/" + neturl.PathEscape(identifier)

	var doc MenuDocument
	err := c.Get(ctx, path, &doc, opts...)
	switch {
	case err == nil:
		// Already nested: skip the nested=1 list round-trip.
		if MenuStats(doc).MaxDepth > 1 {
			return doc, nil
		}
	case !IsNotFound(err):
		return nil, err
	}

	slug := MenuDocumentSlug(doc)
	if slug == "" {
		slug = identifier
	}

	nested, nestedErr := fetchNestedMenuDocument(ctx, c, identifier, slug, opts...)
	if nestedErr != nil {
		// Singular GET already gave us items: keep them rather than failing the read.
		if err == nil && MenuDocumentHasItems(doc) {
			return doc, nil
		}
		return nil, nestedErr
	}
	// Prefer nested list when it carries items (or singular had none).
	if nested != nil && (MenuDocumentHasItems(nested) || !MenuDocumentHasItems(doc)) {
		return nested, nil
	}
	if doc != nil {
		return doc, nil
	}

	return nil, &Error{StatusCode: 404, Message: fmt.Sprintf("menu %q not found", identifier)}
}

func fetchNestedMenuDocument(ctx context.Context, c *Client, identifier, slug string, opts ...RequestOption) (MenuDocument, error) {
	listOpts := append([]RequestOption{}, opts...)
	listOpts = append(listOpts, WithParam("nested", "1"), WithParam("slug", slug))
	var menus []MenuDocument
	if err := c.Get(ctx, "/menus", &menus, listOpts...); err != nil {
		return nil, err
	}
	if selected, ok := SelectMenuDocument(menus, identifier); ok {
		return selected, nil
	}
	if slug != identifier {
		if selected, ok := SelectMenuDocument(menus, slug); ok {
			return selected, nil
		}
	}
	return nil, nil
}

// PatchMenuDocument updates a menu document. Pass WithReplace(true) for full-tree replace semantics.
func PatchMenuDocument(ctx context.Context, c *Client, slug string, doc MenuDocument, opts ...RequestOption) (MenuDocument, error) {
	var out MenuDocument
	path := "/menus/" + neturl.PathEscape(strings.TrimSpace(slug))
	if err := c.Patch(ctx, path, doc, &out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// PostMenuDocument creates a menu document, preserving the nested items tree.
func PostMenuDocument(ctx context.Context, c *Client, doc MenuDocument, opts ...RequestOption) (MenuDocument, error) {
	var out MenuDocument
	if err := c.Post(ctx, "/menus", doc, &out, opts...); err != nil {
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
	stats.HasItems = true
	stats.MaxDepth = menuDepth(items, 1, &stats.ItemCount)
	stats.Shape = menuShape(items)
	return stats
}

// MenuNestingLost reports whether a write response dropped nested depth relative to the submitted tree.
func MenuNestingLost(submitted, returned MenuDocumentStats) bool {
	if !submitted.HasItems && submitted.MaxDepth == 0 && submitted.ItemCount == 0 && submitted.Shape == "" {
		return false
	}
	return returned.MaxDepth < submitted.MaxDepth ||
		returned.ItemCount != submitted.ItemCount ||
		(submitted.Shape != "" && returned.Shape != submitted.Shape)
}

func menuShape(items []any) string {
	encoded, _ := json.Marshal(menuShapeValue(items))
	return string(encoded)
}

func menuShapeValue(items []any) []any {
	shape := make([]any, 0, len(items))
	for index, rawItem := range items {
		item, ok := mapValue(rawItem)
		if !ok {
			continue
		}
		label := stringValue(item["name"])
		if label == "" {
			label = stringValue(item["title"])
		}
		if label == "" {
			label = stringValue(item["id"])
		}
		if label == "" {
			label = fmt.Sprintf("#%d", index)
		}
		children, _ := sliceValue(item["children"])
		shape = append(shape, []any{label, menuShapeValue(children)})
	}
	return shape
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

// MenuItemKinds lists the link kinds the API accepts for a menu item.
var MenuItemKinds = []string{"url", "page", "channel"}

// ErrMenuItemIDsMissing means the current menu was read without item ids, so
// items omitted from a desired tree cannot be tombstoned.
var ErrMenuItemIDsMissing = errors.New("server did not return menu item ids")

// NormalizeMenuDocumentForWrite strips write-unsafe fields, fills API field aliases
// (title→name, url→target_url) without deleting them, and infers an empty kind
// (target_fullpath→page, target_url→url) on a nested menu tree.
func NormalizeMenuDocumentForWrite(doc MenuDocument) {
	items, ok := sliceValue(doc["items"])
	if !ok {
		return
	}
	normalizeMenuItems(items)
}

// ValidateMenuDocumentForWrite rejects menu items whose kind is set but not one
// of MenuItemKinds, naming the item path (e.g. items[1].children[0].kind).
func ValidateMenuDocumentForWrite(doc MenuDocument) error {
	items, _ := sliceValue(doc["items"])
	return validateMenuItems(items, "items")
}

func validateMenuItems(items []any, path string) error {
	for index, rawItem := range items {
		item, ok := mapValue(rawItem)
		if !ok || boolValue(item["_destroy"]) {
			continue
		}
		itemPath := fmt.Sprintf("%s[%d]", path, index)
		if raw, present := item["kind"]; present && raw != nil {
			kind, _ := raw.(string)
			if !slices.Contains(MenuItemKinds, kind) {
				return fmt.Errorf("%s.kind: invalid value %s (allowed: %s)",
					itemPath, jsonString(raw), strings.Join(MenuItemKinds, ", "))
			}
		}
		children, _ := sliceValue(item["children"])
		if err := validateMenuItems(children, itemPath+".children"); err != nil {
			return err
		}
	}
	return nil
}

func jsonString(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(encoded)
}

// ReconcileMenuDocument appends explicit tombstones for existing items omitted
// from a desired full-tree document. This provides replace semantics without
// the server's recursive replace=1 behavior, which can delete nested siblings.
// It returns ErrMenuItemIDsMissing, leaving desired untouched, when any current
// item lacks an id: without ids nothing can be tombstoned, and a PATCH would
// append the desired tree next to the old items.
func ReconcileMenuDocument(current, desired MenuDocument) error {
	if missing, total := countMenuItemsWithoutID(current["items"]); missing > 0 {
		return fmt.Errorf("%w (%d of %d items)", ErrMenuItemIDsMissing, missing, total)
	}
	currentIDs := map[string]struct{}{}
	desiredIDs := map[string]struct{}{}
	collectMenuItemIDs(current["items"], currentIDs)
	collectMenuItemIDs(desired["items"], desiredIDs)

	items, _ := sliceValue(desired["items"])
	setMenuPositions(items)
	removedIDs := make([]string, 0, len(currentIDs))
	for id := range currentIDs {
		if _, retained := desiredIDs[id]; retained {
			continue
		}
		removedIDs = append(removedIDs, id)
	}
	slices.Sort(removedIDs)
	for _, id := range removedIDs {
		items = append(items, map[string]any{"id": id, "_destroy": true})
	}
	desired["items"] = items
	return nil
}

func countMenuItemsWithoutID(raw any) (missing, total int) {
	items, _ := sliceValue(raw)
	for _, rawItem := range items {
		item, ok := mapValue(rawItem)
		if !ok {
			continue
		}
		total++
		if stringValue(item["id"]) == "" {
			missing++
		}
		childMissing, childTotal := countMenuItemsWithoutID(item["children"])
		missing += childMissing
		total += childTotal
	}
	return missing, total
}

func collectMenuItemIDs(raw any, ids map[string]struct{}) {
	items, ok := sliceValue(raw)
	if !ok {
		return
	}
	for _, rawItem := range items {
		item, ok := mapValue(rawItem)
		if !ok {
			continue
		}
		if id := stringValue(item["id"]); id != "" {
			ids[id] = struct{}{}
		}
		collectMenuItemIDs(item["children"], ids)
	}
}

func setMenuPositions(items []any) {
	for position, rawItem := range items {
		item, ok := mapValue(rawItem)
		if !ok {
			continue
		}
		item["position"] = position
		if children, ok := sliceValue(item["children"]); ok {
			setMenuPositions(children)
		}
	}
}

func normalizeMenuItems(items []any) {
	for _, rawItem := range items {
		item, ok := mapValue(rawItem)
		if !ok {
			continue
		}
		delete(item, "target_page")
		copyStringAlias(item, "name", "title")
		copyStringAlias(item, "target_url", "url")
		inferMenuItemKind(item)
		if children, ok := sliceValue(item["children"]); ok && len(children) > 0 {
			normalizeMenuItems(children)
		}
	}
}

// inferMenuItemKind fills an empty kind from the target fields. target_fullpath
// wins because server reads echo a resolved target_url for page items too.
func inferMenuItemKind(item map[string]any) {
	switch kind := item["kind"].(type) {
	case nil:
	case string:
		if strings.TrimSpace(kind) != "" {
			return
		}
	default:
		return // ValidateMenuDocumentForWrite reports it.
	}
	switch {
	case stringValue(item["target_fullpath"]) != "":
		item["kind"] = "page"
	case stringValue(item["target_url"]) != "":
		item["kind"] = "url"
	}
}

// copyStringAlias sets dst from src when dst is empty and src is non-empty.
func copyStringAlias(item map[string]any, dst, src string) {
	if stringValue(item[dst]) != "" {
		return
	}
	if value := stringValue(item[src]); value != "" {
		item[dst] = value
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
