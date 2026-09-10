package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

type surgicalDiffEntry struct {
	Path    string `json:"path"`
	RawPath string `json:"raw_path"`
	Before  any    `json:"before"`
	After   any    `json:"after"`
}

func computeSurgicalDiffs(ops []plannedOp, before, after map[string]any, result *api.BatchResult) ([]surgicalDiffEntry, error) {
	if after == nil && result != nil && len(result.Page) > 0 {
		if err := json.Unmarshal(result.Page, &after); err != nil {
			return nil, fmt.Errorf("decode result page: %w", err)
		}
	}
	var diffs []surgicalDiffEntry
	seen := map[string]struct{}{}
	for i, op := range ops {
		human := op.Human
		if human == "" {
			human = op.Op.Path
		}
		scope := diffScope(op, result, i)
		key := scope + "\x00" + human
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		beforeVal := diffValue(before, op, scope, result, i, true)
		afterVal := diffValue(after, op, scope, result, i, false)
		diffs = append(diffs, surgicalDiffEntry{
			Path:    human,
			RawPath: scope,
			Before:  beforeVal,
			After:   afterVal,
		})
	}
	return diffs, nil
}

func diffScope(op plannedOp, result *api.BatchResult, index int) string {
	switch op.Op.Op {
	case "insert", "delete", "move":
		parent := strings.TrimSuffix(op.Op.Path, "/repeatables")
		if i := strings.LastIndex(parent, "/repeatables/"); i >= 0 {
			if items := strings.LastIndex(parent, "/items/"); items > i {
				parent = parent[:items]
			}
		}
		if op.Op.Op != "insert" {
			if i := strings.LastIndex(op.Op.Path, "/repeatables/"); i >= 0 {
				parent = op.Op.Path[:i]
			}
		}
		return parent + "/repeatables"
	default:
		if op.Op.Path != "" {
			return op.Op.Path
		}
		if result != nil && index < len(result.Results) && result.Results[index].Path != "" {
			return result.Results[index].Path
		}
		return op.Human
	}
}

func diffValue(doc map[string]any, op plannedOp, scope string, result *api.BatchResult, index int, before bool) any {
	if doc == nil {
		return nil
	}
	switch op.Op.Op {
	case "insert", "delete", "move":
		canvasPath := strings.TrimSuffix(scope, "/repeatables")
		node, _ := subtreeAt(doc, canvasPath)
		projected := projectRepeatables(node)
		if !before && op.Op.Op == "insert" && result != nil && index < len(result.Results) {
			if id := result.Results[index].ID; id != "" {
				if block, ok := subtreeAt(doc, canvasPath+"/repeatables/"+id); ok {
					return map[string]any{"repeatables": projected, "inserted": stripNoise(block)}
				}
			}
		}
		return projected
	default:
		node, _ := subtreeAt(doc, scope)
		return stripNoise(node)
	}
}

func projectRepeatables(node any) []map[string]any {
	item, _ := node.(map[string]any)
	if item == nil {
		return nil
	}
	refs := siblingRefs(item)
	out := make([]map[string]any, len(refs))
	for i, ref := range refs {
		out[i] = map[string]any{"index": ref.Index, "id": ref.ID, "slug": ref.Slug}
	}
	return out
}

func printSurgicalDiffs(ctx context.Context, diffs []surgicalDiffEntry) error {
	styles := newDiffStyles(ctx)
	for _, entry := range diffs {
		beforeText, err := stableJSON(entry.Before)
		if err != nil {
			return err
		}
		afterText, err := stableJSON(entry.After)
		if err != nil {
			return err
		}
		if beforeText == afterText {
			if _, err := output.Fprintln(ctx, "(no change)"); err != nil {
				return err
			}
			continue
		}
		label := entry.Path
		if label == "" {
			label = entry.RawPath
		}
		diff := output.Unified("before/"+label, "after/"+label, beforeText, afterText)
		if _, err := output.Fprintf(ctx, "%s", styles.colorize(diff)); err != nil {
			return err
		}
	}
	return nil
}

func stripNoise(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for key, value := range t {
			if key == "created_at" || key == "updated_at" {
				continue
			}
			out[key] = stripNoise(value)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, value := range t {
			out[i] = stripNoise(value)
		}
		return out
	default:
		return v
	}
}

func stableJSON(v any) (string, error) {
	if v == nil {
		return "null\n", nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var normalized any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&normalized); err != nil {
		return "", err
	}
	normalized = sortJSON(normalized)
	out, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}

func sortJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for key := range t {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(t))
		for _, key := range keys {
			out[key] = sortJSON(t[key])
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, value := range t {
			out[i] = sortJSON(value)
		}
		return out
	default:
		return v
	}
}
