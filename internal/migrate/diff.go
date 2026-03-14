package migrate

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

// DiffChange describes one normalized difference.
type DiffChange struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	From any    `json:"from,omitempty"`
	To   any    `json:"to,omitempty"`
}

// DiffSet groups added, removed, and updated changes.
type DiffSet struct {
	Added   []DiffChange `json:"added,omitempty"`
	Removed []DiffChange `json:"removed,omitempty"`
	Updated []DiffChange `json:"updated,omitempty"`
}

// HasChanges reports whether any differences exist.
func (d DiffSet) HasChanges() bool {
	return len(d.Added) > 0 || len(d.Removed) > 0 || len(d.Updated) > 0
}

// DiffNormalized compares two normalized values.
func DiffNormalized(from, to any) DiffSet {
	var out DiffSet
	diffValue("", from, to, &out)
	sortChanges(out.Added)
	sortChanges(out.Removed)
	sortChanges(out.Updated)
	return out
}

// NormalizeChannel strips unstable fields from a channel detail.
func NormalizeChannel(detail api.ChannelDetail) map[string]any {
	var raw map[string]any
	data, _ := json.Marshal(detail)
	_ = json.Unmarshal(data, &raw)
	delete(raw, "id")
	delete(raw, "customizations")
	return raw
}

// NormalizeCustomizations strips unstable fields from a customization slice.
func NormalizeCustomizations(fields []api.CustomField) []map[string]any {
	items := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		items = append(items, normalizeField(field))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return fmt.Sprint(items[i]["name"]) < fmt.Sprint(items[j]["name"])
	})
	return items
}

// normalizeField strips source IDs from a single custom field (and its select options).
func normalizeField(field api.CustomField) map[string]any {
	var raw map[string]any
	data, _ := json.Marshal(field)
	_ = json.Unmarshal(data, &raw)
	delete(raw, "id")
	if options, ok := raw["select_options"].([]any); ok {
		for _, option := range options {
			if optionMap, ok := option.(map[string]any); ok {
				delete(optionMap, "id")
			}
		}
	}
	return raw
}

func diffValue(path string, from, to any, out *DiffSet) {
	from = normalizeJSONValue(from)
	to = normalizeJSONValue(to)
	switch {
	case from == nil && to == nil:
		return
	case from == nil:
		out.Added = append(out.Added, DiffChange{Kind: "added", Path: coalescePath(path), To: to})
		return
	case to == nil:
		out.Removed = append(out.Removed, DiffChange{Kind: "removed", Path: coalescePath(path), From: from})
		return
	}

	switch fromTyped := from.(type) {
	case map[string]any:
		toTyped, ok := to.(map[string]any)
		if !ok {
			out.Updated = append(out.Updated, DiffChange{Kind: "updated", Path: coalescePath(path), From: from, To: to})
			return
		}
		keys := map[string]struct{}{}
		for key := range fromTyped {
			keys[key] = struct{}{}
		}
		for key := range toTyped {
			keys[key] = struct{}{}
		}
		keyList := make([]string, 0, len(keys))
		for key := range keys {
			keyList = append(keyList, key)
		}
		sort.Strings(keyList)
		for _, key := range keyList {
			diffValue(joinPath(path, key), fromTyped[key], toTyped[key], out)
		}
	case []any:
		toTyped, ok := to.([]any)
		if !ok {
			out.Updated = append(out.Updated, DiffChange{Kind: "updated", Path: coalescePath(path), From: from, To: to})
			return
		}
		maxLen := len(fromTyped)
		if len(toTyped) > maxLen {
			maxLen = len(toTyped)
		}
		for idx := 0; idx < maxLen; idx++ {
			var left any
			var right any
			if idx < len(fromTyped) {
				left = fromTyped[idx]
			}
			if idx < len(toTyped) {
				right = toTyped[idx]
			}
			diffValue(joinPath(path, fmt.Sprintf("[%d]", idx)), left, right, out)
		}
	default:
		if !reflect.DeepEqual(from, to) {
			out.Updated = append(out.Updated, DiffChange{Kind: "updated", Path: coalescePath(path), From: from, To: to})
		}
	}
}

func normalizeJSONValue(value any) any {
	switch typed := value.(type) {
	case []map[string]any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, normalizeJSONValue(item))
		}
		return items
	case []api.CustomField:
		items := make([]any, 0, len(typed))
		for _, item := range NormalizeCustomizations(typed) {
			items = append(items, item)
		}
		return items
	case []string:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items
	default:
		return value
	}
}

func sortChanges(changes []DiffChange) {
	sort.SliceStable(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})
}

func joinPath(base, next string) string {
	if strings.HasPrefix(next, "[") {
		return base + next
	}
	if base == "" {
		return next
	}
	return base + "." + next
}

func coalescePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "$"
	}
	return path
}
