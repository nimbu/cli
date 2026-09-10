package cmd

import (
	"fmt"
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
	if resolved.Kind == pagepath.KindCanvas || resolved.Kind == pagepath.KindRepeatable {
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
