package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nimbu/cli/internal/pagepath"
)

func enrichResolved(resolved pagepath.Resolved, doc map[string]any) pagepath.Resolved {
	if resolved.Kind == pagepath.KindCanvas && len(resolved.Siblings) == 0 {
		if item, ok := resolved.Value.(map[string]any); ok {
			resolved.Siblings = siblingRefs(item)
		}
	}
	if resolved.Kind != "" && resolved.Type != "" {
		return resolved
	}
	raw := strings.TrimSuffix(resolved.RawPath, "/repeatables")
	node, ok := subtreeAt(doc, raw)
	if !ok {
		return resolved
	}
	if resolved.RawPath == raw && isPageFieldRaw(raw) {
		resolved.Kind = pagepath.KindField
		resolved.Type = "page"
		resolved.Value = node
		return resolved
	}
	item, _ := node.(map[string]any)
	typ := stringAny(item["type"])
	switch {
	case isRepeatableRaw(resolved.RawPath):
		resolved.Kind = pagepath.KindRepeatable
		resolved.Type = stringAny(item["slug"])
		resolved.RepeatableID = stringAny(item["id"])
		resolved.RepeatableSlug = resolved.Type
		// A raw path carries no siblings or position: read them off the canvas.
		if idx := strings.LastIndex(raw, "/repeatables/"); idx > 0 {
			parentRaw := raw[:idx]
			parent, _ := subtreeAt(doc, parentRaw)
			if siblings := siblingRefs(asMapAny(parent)); len(siblings) > 0 {
				resolved.ParentCanvasRaw = parentRaw
				resolved.Siblings = siblings
				for _, sibling := range siblings {
					if sibling.ID == resolved.RepeatableID {
						resolved.Index = sibling.Index
						break
					}
				}
			}
		}
	case typ == "canvas" || strings.HasSuffix(resolved.RawPath, "/repeatables"):
		resolved.Kind = pagepath.KindCanvas
		resolved.Type = "canvas"
		if resolved.ParentCanvasRaw == "" {
			resolved.ParentCanvasRaw = strings.TrimSuffix(raw, "/repeatables")
		}
	case isPageFieldRaw(raw):
		resolved.Kind = pagepath.KindField
		resolved.Type = "page"
	default:
		resolved.Kind = pagepath.KindItem
		resolved.Type = typ
	}
	if resolved.Value == nil {
		resolved.Value = node
	}
	if len(resolved.Siblings) == 0 && (resolved.Kind == pagepath.KindCanvas || resolved.Kind == pagepath.KindRepeatable) {
		if parent := canvasNode(doc, raw); parent != nil {
			resolved.Siblings = siblingRefs(parent)
		}
	}
	return resolved
}

func isRepeatableRaw(raw string) bool {
	idx := strings.LastIndex(raw, "/repeatables/")
	if idx < 0 {
		return false
	}
	rest := raw[idx+len("/repeatables/"):]
	return rest != "" && !strings.Contains(rest, "/")
}

func isPageFieldRaw(raw string) bool {
	field := strings.TrimPrefix(raw, "/")
	return !strings.Contains(field, "/") && field != "items"
}

func canvasNode(doc map[string]any, raw string) map[string]any {
	path := strings.TrimSuffix(raw, "/repeatables")
	if strings.Contains(path, "/repeatables/") {
		if i := strings.LastIndex(path, "/repeatables/"); i >= 0 {
			if items := strings.LastIndex(path, "/items/"); items > i {
				path = path[:items]
			}
		}
	}
	node, _ := subtreeAt(doc, path)
	item, _ := node.(map[string]any)
	return item
}

func siblingRefs(canvas map[string]any) []pagepath.RepeatableRef {
	if canvas == nil {
		return nil
	}
	raw, _ := canvas["repeatables"].([]any)
	type pair struct {
		ref pagepath.RepeatableRef
		pos float64
	}
	pairs := make([]pair, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		pairs = append(pairs, pair{
			ref: pagepath.RepeatableRef{ID: stringAny(m["id"]), Slug: stringAny(m["slug"])},
			pos: numberAny(m["position"]),
		})
	}
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].pos < pairs[i].pos {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
	out := make([]pagepath.RepeatableRef, len(pairs))
	for i, p := range pairs {
		p.ref.Index = i
		out[i] = p.ref
	}
	return out
}

func subtreeAt(doc map[string]any, raw string) (any, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/" {
		return doc, true
	}
	current := any(doc)
	parts := strings.Split(strings.TrimPrefix(raw, "/"), "/")
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		switch part {
		case "items":
			next, ok := m["items"]
			if !ok {
				return nil, false
			}
			current = next
		case "repeatables":
			reps, ok := m["repeatables"]
			if !ok {
				return nil, false
			}
			if i+1 >= len(parts) {
				return reps, true
			}
			i++
			id := parts[i]
			list, ok := reps.([]any)
			if !ok {
				return nil, false
			}
			found := false
			for _, rep := range list {
				rm, ok := rep.(map[string]any)
				if !ok {
					continue
				}
				if stringAny(rm["id"]) == id {
					current = rm
					found = true
					break
				}
			}
			if !found {
				return nil, false
			}
		default:
			name, err := pagepath.UnescapeName(part)
			if err != nil {
				name = part
			}
			next, ok := m[name]
			if !ok {
				return nil, false
			}
			current = next
		}
	}
	return current, true
}

func numberAny(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case jsonNumber:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

type jsonNumber interface {
	Float64() (float64, error)
}

func requireResolvedKind(resolved pagepath.Resolved, input string, kind pagepath.Kind, candidates []string) error {
	if resolved.Kind == kind {
		return nil
	}
	msg := fmt.Sprintf("path %q: expected %s, got %s", input, kind, resolved.Kind)
	if len(candidates) > 0 {
		msg += "; candidates:\n  " + strings.Join(candidates, "\n  ")
	}
	return newDetailedError(fmt.Errorf("%s", msg), errorRequestInvalid, ExitUsage, map[string]any{
		"path":       input,
		"candidates": candidates,
	})
}

func canvasCandidates(doc map[string]any) []string {
	items, _ := doc["items"].(map[string]any)
	var out []string
	for name, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || stringAny(item["type"]) != "canvas" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func formatRepeatableCandidates(siblings []pagepath.RepeatableRef) []string {
	out := make([]string, len(siblings))
	for i, sib := range siblings {
		out[i] = fmt.Sprintf("[%d] %s %s", sib.Index, sib.Slug, sib.ID)
	}
	return out
}

// resolveUserPath resolves a human path, or a raw path that is first
// normalized against the document: shortened forms are expanded to the full
// grammar and positional segments are rewritten to repeatable ids.
func (s *surgicalSession) resolveUserPath(input string) (pagepath.Resolved, error) {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "/") {
		normalized, err := normalizeRawRepeatableIndexes(s.doc, trimmed)
		if err != nil {
			return pagepath.Resolved{}, err
		}
		trimmed = normalized
	}
	return s.resolve(trimmed)
}

// batchOpNames are the operations the page batch endpoint accepts.
var batchOpNames = []string{"set", "insert", "delete", "move"}

func validateBatchOpName(op string) error {
	for _, name := range batchOpNames {
		if op == name {
			return nil
		}
	}
	return newDetailedError(
		fmt.Errorf("unknown op %q; valid ops: %s", op, strings.Join(batchOpNames, ", ")),
		errorRequestInvalid, ExitUsage,
		map[string]any{"op": op, "valid_ops": batchOpNames},
	)
}

// validateBatchOpPath rejects paths that append a /content property segment to
// an editable (…/items/<name>/content). An editable literally named "content"
// (/items/content) is a legitimate path and is left alone.
func validateBatchOpPath(path string) error {
	trimmed := strings.TrimSpace(path)
	if !strings.HasSuffix(trimmed, "/content") {
		return nil
	}
	segments := strings.Split(strings.Trim(trimmed, "/"), "/")
	// …/items/<editable>/content is the rejected form: the segment two before
	// the trailing "content" is the "items" collection.
	if len(segments) < 3 || segments[len(segments)-3] != "items" {
		return nil
	}
	return newDetailedError(
		fmt.Errorf("path %q ends in /content; paths end at the editable name", path),
		errorRequestInvalid, ExitUsage,
		map[string]any{"path": path, "hint": "paths end at the editable name"},
	)
}

// insertRepeatablesPath normalizes a canvas path so that it addresses the
// canvas repeatables collection, which is what the insert op requires.
func insertRepeatablesPath(raw string) string {
	return strings.TrimSuffix(strings.TrimRight(raw, "/"), "/repeatables") + "/repeatables"
}

// rawPathGrammar is the full form a raw API path must take, shown whenever a
// raw path cannot be walked against the page document.
const rawPathGrammar = "expected /items/<canvas>/repeatables/<id>/items/<editable>" +
	"[/repeatables/<id>/items/<editable>] or /<page field>"

// normalizeRawRepeatableIndexes normalizes a raw API path against the loaded
// page document. Kept under its original name for its callers; see
// normalizeRawPath.
func normalizeRawRepeatableIndexes(doc map[string]any, raw string) (string, error) {
	return normalizeRawPath(doc, raw)
}

// normalizeRawPath walks a raw path against the page document and returns the
// canonical full-form path. It repairs the shortened forms people write by
// hand — a missing leading /items, a missing /repeatables before a repeatable
// id, a missing /items before a nested editable — and rewrites positional
// indexes to repeatable ids. Every canvas, editable and id it walks past is
// checked against the document, so a wrong path fails here instead of at the
// API. A path that is already in full form comes back byte-identical.
func normalizeRawPath(doc map[string]any, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	parts := splitRawPath(trimmed)
	if len(parts) == 0 {
		return raw, nil
	}
	items := asMapAny(doc["items"])

	i := 0
	if parts[0] == "items" {
		i = 1
		if i == len(parts) {
			return "", rawPathError(trimmed,
				fmt.Sprintf("path %q: /items addresses no editable", trimmed),
				"editables", editableCandidates(items))
		}
	} else if _, ok := items[unescapeSegment(parts[0])]; !ok {
		// Not a top-level editable: the only other legal first segment is a
		// single page field.
		field := unescapeSegment(parts[0])
		if len(parts) == 1 && (pagepath.IsPageField(field) || hasKeyAny(doc, field)) {
			return "/" + parts[0], nil
		}
		return "", rawPathError(trimmed,
			fmt.Sprintf("path %q: %q is neither a page field nor a top-level editable", trimmed, parts[0]),
			"page fields and editables", pageCandidates(doc, items))
	}

	out := []string{"items"}
	canvasDepth := 0
	for i < len(parts) {
		seg := parts[i]
		name := unescapeSegment(seg)
		node, ok := items[name]
		if !ok {
			return "", rawPathError(trimmed,
				fmt.Sprintf("path %q: editable %q not found in %s", trimmed, name, displayRawPath("/"+strings.Join(out, "/"))),
				"editables", editableCandidates(items))
		}
		out = append(out, seg)
		i++
		if i == len(parts) {
			return "/" + strings.Join(out, "/"), nil
		}

		canvas := asMapAny(node)
		if stringAny(canvas["type"]) != "canvas" {
			return "", rawPathError(trimmed,
				fmt.Sprintf("path %q: %s is not a canvas, so the path cannot descend into it", trimmed, name),
				"", nil)
		}
		if parts[i] == "repeatables" {
			i++
		}
		out = append(out, "repeatables")
		if i == len(parts) {
			return "/" + strings.Join(out, "/"), nil
		}

		canvasDepth++
		if canvasDepth > 2 {
			return "", rawPathError(trimmed,
				fmt.Sprintf("path %q: path exceeds two canvas levels", trimmed), "", nil)
		}
		canvasRaw := "/" + strings.Join(out[:len(out)-1], "/")
		id, err := repeatableSegmentID(parts[i], trimmed, canvasRaw, siblingRefs(canvas))
		if err != nil {
			return "", err
		}
		out = append(out, id)
		i++
		if i == len(parts) {
			return "/" + strings.Join(out, "/"), nil
		}

		if parts[i] == "items" {
			i++
		}
		out = append(out, "items")
		repeatable, _ := subtreeAt(doc, "/"+strings.Join(out[:len(out)-1], "/"))
		items = asMapAny(asMapAny(repeatable)["items"])
		if i == len(parts) {
			return "", rawPathError(trimmed,
				fmt.Sprintf("path %q: path stops at a repeatable's items; append the editable name", trimmed),
				"editables", editableCandidates(items))
		}
	}
	return "/" + strings.Join(out, "/"), nil
}

func splitRawPath(raw string) []string {
	out := make([]string, 0, 8)
	for _, part := range strings.Split(raw, "/") {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func unescapeSegment(segment string) string {
	name, err := pagepath.UnescapeName(segment)
	if err != nil {
		return segment
	}
	return name
}

func hasKeyAny(doc map[string]any, key string) bool {
	_, ok := doc[key]
	return ok
}

func editableCandidates(items map[string]any) []string {
	out := make([]string, 0, len(items))
	for name, raw := range items {
		if typ := stringAny(asMapAny(raw)["type"]); typ != "" {
			out = append(out, name+" ("+typ+")")
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func pageCandidates(doc map[string]any, items map[string]any) []string {
	out := editableCandidates(items)
	for key := range doc {
		if key == "items" || !pagepath.IsPageField(key) {
			continue
		}
		out = append(out, key+" (page field)")
	}
	sort.Strings(out)
	return out
}

func rawPathError(path, msg, candidateLabel string, candidates []string) error {
	if len(candidates) > 0 && candidateLabel != "" {
		msg += "; " + candidateLabel + ":\n  " + strings.Join(candidates, "\n  ")
	}
	msg += "\n" + rawPathGrammar
	return newDetailedError(fmt.Errorf("%s", msg), errorRequestInvalid, ExitUsage, map[string]any{
		"path":       path,
		"candidates": candidates,
	})
}

func repeatableSegmentID(segment, raw, canvasRaw string, siblings []pagepath.RepeatableRef) (string, error) {
	for _, sibling := range siblings {
		if sibling.ID == segment {
			return segment, nil
		}
	}
	index, err := strconv.Atoi(segment)
	if err != nil {
		canvas := displayRawPath(canvasRaw)
		candidates := formatRepeatableCandidates(siblings)
		msg := fmt.Sprintf("path %q: repeatable id %q not found in %s", raw, segment, canvas)
		if len(siblings) == 0 {
			msg += " (no repeatables yet)"
		}
		return "", rawPathError(raw, msg, "repeatables", candidates)
	}
	for _, sibling := range siblings {
		if sibling.Index == index {
			return sibling.ID, nil
		}
	}
	canvas := displayRawPath(canvasRaw)
	candidates := formatRepeatableCandidates(siblings)
	var msg string
	switch {
	case len(siblings) == 0:
		msg = fmt.Sprintf("path %q: %q is a position, not a repeatable id, and %s has no repeatables", raw, segment, canvas)
	default:
		msg = fmt.Sprintf("path %q: index %d is out of range for %s (%d repeatables):\n  %s",
			raw, index, canvas, len(siblings), strings.Join(candidates, "\n  "))
	}
	return "", newDetailedError(fmt.Errorf("%s", msg), errorRequestInvalid, ExitUsage, map[string]any{
		"path":       raw,
		"candidates": candidates,
	})
}
