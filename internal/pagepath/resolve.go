package pagepath

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Kind classifies a resolved path.
type Kind string

const (
	KindField      Kind = "field"
	KindItem       Kind = "item"
	KindCanvas     Kind = "canvas"
	KindRepeatable Kind = "repeatable"
)

// Resolved is a human path mapped onto a page document.
type Resolved struct {
	RawPath         string
	Kind            Kind
	Type            string
	RepeatableID    string
	RepeatableSlug  string
	Index           int
	ParentCanvasRaw string
	Siblings        []RepeatableRef
	Value           any
}

type namedType struct {
	Name string
	Type string
}

type repeatable struct {
	ref   RepeatableRef
	value map[string]any
}

// Resolve walks p against the page document. schema may be nil.
func Resolve(p Path, page map[string]any, schema *Schema) (Resolved, error) {
	if p.Raw != "" && p.Field == "" && len(p.Segments) == 0 {
		return Resolved{RawPath: p.Raw}, nil
	}
	if p.Field != "" {
		return Resolved{RawPath: "/" + p.Field, Kind: KindField, Type: "page", Value: page[p.Field]}, nil
	}

	items, _ := asMap(page["items"])
	human := p.String()
	current := items
	raw := ""
	canvasDepth := 0
	repID, repSlug := "", ""
	index := 0
	parentCanvas := ""
	var siblings []RepeatableRef
	schemaCanvas := ""

	for i, seg := range p.Segments {
		last := i == len(p.Segments)-1
		item, ok := asMap(current[seg.Name])
		if !ok {
			if i == 0 {
				return Resolved{}, pageEditableError(human, seg.Name, page)
			}
			context := fmt.Sprintf("%s[%d] (%s)", schemaCanvas, index, repSlug)
			return Resolved{}, editableError(human, seg.Name, context, current, schema, schemaCanvas, repSlug)
		}

		typ := asString(item["type"])
		raw = joinItem(raw, seg.Name)
		if typ != "canvas" {
			if seg.Selector != nil {
				return Resolved{}, &ResolveError{Path: human, Kind: ResolveKindEditable, msg: fmt.Sprintf("path %q: %s is not a canvas", human, seg.Name)}
			}
			if !last {
				return Resolved{}, &ResolveError{Path: human, Kind: ResolveKindEditable, msg: fmt.Sprintf("path %q: cannot descend into %s", human, seg.Name)}
			}
			return Resolved{
				RawPath: raw, Kind: KindItem, Type: typ,
				RepeatableID: repID, RepeatableSlug: repSlug, Index: index,
				ParentCanvasRaw: parentCanvas, Siblings: siblings, Value: item,
			}, nil
		}

		if seg.Selector == nil {
			if !last {
				return Resolved{}, canvasNeedsSelector(human, seg.Name, item)
			}
			return Resolved{
				RawPath: raw, Kind: KindCanvas, Type: typ,
				RepeatableID: repID, RepeatableSlug: repSlug, Index: index,
				ParentCanvasRaw: parentCanvas, Siblings: siblings, Value: item,
			}, nil
		}
		if canvasDepth >= 2 {
			return Resolved{}, &ResolveError{Path: human, Kind: ResolveKindDepth, msg: fmt.Sprintf("path %q: path exceeds two canvas levels", human)}
		}
		canvasDepth++
		reps := orderedRepeatables(item)
		picked, err := pickRepeatable(human, seg.Name, *seg.Selector, reps, schema)
		if err != nil {
			return Resolved{}, err
		}
		parentCanvas = raw
		raw = raw + "/repeatables/" + picked.ref.ID
		repID, repSlug, index = picked.ref.ID, picked.ref.Slug, picked.ref.Index
		siblings = refsOf(reps)
		schemaCanvas = seg.Name
		if last {
			return Resolved{
				RawPath: raw, Kind: KindRepeatable, Type: picked.ref.Slug,
				RepeatableID: repID, RepeatableSlug: repSlug, Index: index,
				ParentCanvasRaw: parentCanvas, Siblings: siblings, Value: picked.value,
			}, nil
		}
		next, _ := asMap(picked.value["items"])
		current = next
	}

	return Resolved{}, &ResolveError{Path: human, msg: fmt.Sprintf("path %q: empty path", human)}
}

func joinItem(raw, name string) string {
	if raw == "" {
		return "/items/" + EscapeName(name)
	}
	return raw + "/items/" + EscapeName(name)
}

func orderedRepeatables(canvas map[string]any) []repeatable {
	raw, _ := asSlice(canvas["repeatables"])
	out := make([]repeatable, 0, len(raw))
	for _, item := range raw {
		m, ok := asMap(item)
		if !ok {
			continue
		}
		out = append(out, repeatable{
			value: m,
			ref: RepeatableRef{
				ID:   asString(m["id"]),
				Slug: asString(m["slug"]),
			},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return positionOf(out[i].value) < positionOf(out[j].value)
	})
	for i := range out {
		out[i].ref.Index = i
	}
	return out
}

func positionOf(m map[string]any) float64 {
	switch v := m["position"].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}

func pickRepeatable(human, canvas string, sel Selector, reps []repeatable, schema *Schema) (repeatable, error) {
	sibs := refsOf(reps)
	switch sel.Kind {
	case SelID:
		for _, r := range reps {
			if r.ref.ID == sel.ID {
				return r, nil
			}
		}
		return repeatable{}, repeatableIDError(human, sel.ID, canvas, sibs)
	case SelSlug:
		var matches []repeatable
		for _, r := range reps {
			if r.ref.Slug == sel.Slug {
				matches = append(matches, r)
			}
		}
		if len(matches) == 0 {
			return repeatable{}, repeatableIDError(human, sel.Slug, canvas, sibs)
		}
		if len(matches) > 1 {
			cands := formatRepeatables(refsOf(matches))
			return repeatable{}, &ResolveError{
				Path:       human,
				Kind:       ResolveKindSlug,
				Candidates: cands,
				msg: fmt.Sprintf("path %q: slug %q matches %d repeatables; use an index or id=:\n  %s",
					human, sel.Slug, len(matches), strings.Join(cands, ", ")),
			}
		}
		return matches[0], nil
	default:
		if sel.Index < 0 || sel.Index >= len(reps) {
			return repeatable{}, indexError(human, canvas, sibs, schema)
		}
		return reps[sel.Index], nil
	}
}

func refsOf(reps []repeatable) []RepeatableRef {
	out := make([]RepeatableRef, len(reps))
	for i, r := range reps {
		out[i] = r.ref
	}
	return out
}

func formatRepeatables(sibs []RepeatableRef) []string {
	out := make([]string, len(sibs))
	for i, s := range sibs {
		out[i] = fmt.Sprintf("[%d] %s %s", s.Index, s.Slug, s.ID)
	}
	return out
}

func indexError(human, canvas string, sibs []RepeatableRef, schema *Schema) error {
	cands := formatRepeatables(sibs)
	var b strings.Builder
	if len(sibs) == 0 {
		fmt.Fprintf(&b, "path %q: %s has no repeatables yet; add one with: nimbu pages insert --page <page> --path %s --slug <slug>", human, canvas, canvas)
		if slugs := schema.BlockSlugs(canvas); len(slugs) > 0 {
			fmt.Fprintf(&b, "; allowed slugs: %s", strings.Join(slugs, ", "))
		}
	} else {
		fmt.Fprintf(&b, "path %q: %s has %d repeatables (indexes 0-%d):", human, canvas, len(sibs), len(sibs)-1)
		for _, line := range cands {
			b.WriteByte('\n')
			b.WriteString("  ")
			b.WriteString(line)
		}
	}
	return &ResolveError{Path: human, Kind: ResolveKindIndex, Candidates: cands, msg: b.String()}
}

func repeatableIDError(human, id, canvas string, sibs []RepeatableRef) error {
	cands := formatRepeatables(sibs)
	var msg string
	if canvas != "" {
		msg = fmt.Sprintf("path %q: repeatable id %q not found in %s; repeatables: %s", human, id, canvas, strings.Join(cands, ", "))
	} else {
		msg = fmt.Sprintf("path %q: repeatable id %q not found; repeatables: %s", human, id, strings.Join(cands, ", "))
	}
	return &ResolveError{Path: human, Kind: ResolveKindID, Candidates: cands, msg: msg}
}

func canvasNeedsSelector(human, canvas string, item map[string]any) error {
	cands := formatRepeatables(refsOf(orderedRepeatables(item)))
	return &ResolveError{
		Path:       human,
		Kind:       ResolveKindIndex,
		Candidates: cands,
		msg:        fmt.Sprintf("path %q: %s is a canvas; choose a repeatable:\n  %s", human, canvas, strings.Join(cands, "\n  ")),
	}
}

func editableError(human, name, context string, items map[string]any, schema *Schema, canvas, slug string) error {
	cands := formatNamed(collectEditables(items, schema, canvas, slug))
	return &ResolveError{
		Path:       human,
		Kind:       ResolveKindEditable,
		Candidates: cands,
		msg:        fmt.Sprintf("path %q: editable %q not found in %s; editables:\n  %s", human, name, context, strings.Join(cands, ", ")),
	}
}

func pageEditableError(human, name string, page map[string]any) error {
	cands := formatNamed(collectPageEditables(page))
	return &ResolveError{
		Path:       human,
		Kind:       ResolveKindPage,
		Candidates: cands,
		msg:        fmt.Sprintf("path %q: editable %q not found on page; editables:\n  %s", human, name, strings.Join(cands, ", ")),
	}
}

func collectEditables(items map[string]any, schema *Schema, canvas, slug string) []namedType {
	seen := map[string]namedType{}
	for name, raw := range items {
		seen[name] = namedType{Name: name, Type: typeOf(raw)}
	}
	for _, field := range schema.fieldsFor(canvas, slug) {
		if _, ok := seen[field.Slug]; !ok {
			seen[field.Slug] = namedType{Name: field.Slug, Type: field.Type}
		}
	}
	return sortNamed(seen)
}

func collectPageEditables(page map[string]any) []namedType {
	seen := map[string]namedType{}
	if items, ok := asMap(page["items"]); ok {
		for name, raw := range items {
			seen[name] = namedType{Name: name, Type: typeOf(raw)}
		}
	}
	for _, field := range pageFieldOrder {
		if _, ok := page[field]; ok {
			seen[field] = namedType{Name: field, Type: "page field"}
		}
	}
	return sortNamed(seen)
}

func sortNamed(seen map[string]namedType) []namedType {
	out := make([]namedType, 0, len(seen))
	for _, n := range seen {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func formatNamed(items []namedType) []string {
	out := make([]string, len(items))
	for i, n := range items {
		out[i] = n.Name + " (" + n.Type + ")"
	}
	return out
}

func typeOf(v any) string {
	m, ok := asMap(v)
	if !ok {
		return ""
	}
	return asString(m["type"])
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func asString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
