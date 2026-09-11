package pagepath

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePageField(t *testing.T) {
	page := loadPage(t)
	p, err := Parse("title")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(p, page, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.RawPath != "/title" || got.Kind != KindField || got.Type != "page" {
		t.Fatalf("resolved = %#v", got)
	}
	if got.Value != "Home" {
		t.Fatalf("value = %#v", got.Value)
	}
}

func TestResolveRawPassthrough(t *testing.T) {
	page := loadPage(t)
	raw := "/items/Blokken/repeatables/000000000000000000000001/items/Title"
	p, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(p, page, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.RawPath != raw {
		t.Fatalf("RawPath = %q", got.RawPath)
	}
}

func TestResolveGrammarForms(t *testing.T) {
	page := loadPage(t)
	tests := []struct {
		name   string
		input  string
		raw    string
		kind   Kind
		typ    string
		id     string
		slug   string
		index  int
		parent string
	}{
		{
			name:  "canvas",
			input: "Blokken",
			raw:   "/items/Blokken",
			kind:  KindCanvas,
			typ:   "canvas",
		},
		{
			name:   "repeatable by reordered index",
			input:  "Blokken[0]",
			raw:    "/items/Blokken/repeatables/000000000000000000000001",
			kind:   KindRepeatable,
			typ:    "hero_stage",
			id:     "000000000000000000000001",
			slug:   "hero_stage",
			index:  0,
			parent: "/items/Blokken",
		},
		{
			name:   "array order differs from position",
			input:  "Blokken[2]",
			raw:    "/items/Blokken/repeatables/000000000000000000000003",
			kind:   KindRepeatable,
			typ:    "case_cards",
			id:     "000000000000000000000003",
			slug:   "case_cards",
			index:  2,
			parent: "/items/Blokken",
		},
		{
			name:   "id selector",
			input:  "Blokken[id=000000000000000000000002]",
			raw:    "/items/Blokken/repeatables/000000000000000000000002",
			kind:   KindRepeatable,
			typ:    "proof_strip",
			id:     "000000000000000000000002",
			slug:   "proof_strip",
			index:  1,
			parent: "/items/Blokken",
		},
		{
			name:   "unique slug",
			input:  "Blokken[slug=hero_stage]",
			raw:    "/items/Blokken/repeatables/000000000000000000000001",
			kind:   KindRepeatable,
			typ:    "hero_stage",
			id:     "000000000000000000000001",
			slug:   "hero_stage",
			index:  0,
			parent: "/items/Blokken",
		},
		{
			name:   "nested field",
			input:  "Blokken[0].Title",
			raw:    "/items/Blokken/repeatables/000000000000000000000001/items/Title",
			kind:   KindItem,
			typ:    "text",
			id:     "000000000000000000000001",
			slug:   "hero_stage",
			index:  0,
			parent: "/items/Blokken",
		},
		{
			name:   "spaces in name",
			input:  "Blokken[0].Left Button - Link",
			raw:    "/items/Blokken/repeatables/000000000000000000000001/items/Left%20Button%20-%20Link",
			kind:   KindItem,
			typ:    "field",
			id:     "000000000000000000000001",
			slug:   "hero_stage",
			index:  0,
			parent: "/items/Blokken",
		},
		{
			name:   "parentheses in name",
			input:  "Blokken[0].Swoosh Bottom (hero)",
			raw:    "/items/Blokken/repeatables/000000000000000000000001/items/Swoosh%20Bottom%20%28hero%29",
			kind:   KindItem,
			typ:    "file",
			id:     "000000000000000000000001",
			slug:   "hero_stage",
			index:  0,
			parent: "/items/Blokken",
		},
		{
			name:  "en dash top-level item",
			input: "Korte uitleg \u2013 vraag",
			raw:   "/items/Korte%20uitleg%20%E2%80%93%20vraag",
			kind:  KindItem,
			typ:   "text",
		},
		{
			name:   "nested canvas",
			input:  "Blokken[2].Photos",
			raw:    "/items/Blokken/repeatables/000000000000000000000003/items/Photos",
			kind:   KindCanvas,
			typ:    "canvas",
			id:     "000000000000000000000003",
			slug:   "case_cards",
			index:  2,
			parent: "/items/Blokken",
		},
		{
			name:   "nested photos ordered by position",
			input:  "Blokken[2].Photos[0]",
			raw:    "/items/Blokken/repeatables/000000000000000000000003/items/Photos/repeatables/000000000000000000000010",
			kind:   KindRepeatable,
			typ:    "image",
			id:     "000000000000000000000010",
			slug:   "image",
			index:  0,
			parent: "/items/Blokken/repeatables/000000000000000000000003/items/Photos",
		},
		{
			name:   "two canvas levels",
			input:  "Blokken[2].Photos[0].Image",
			raw:    "/items/Blokken/repeatables/000000000000000000000003/items/Photos/repeatables/000000000000000000000010/items/Image",
			kind:   KindItem,
			typ:    "file",
			id:     "000000000000000000000010",
			slug:   "image",
			index:  0,
			parent: "/items/Blokken/repeatables/000000000000000000000003/items/Photos",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Parse(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Resolve(p, page, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.RawPath != tt.raw {
				t.Fatalf("RawPath = %q, want %q", got.RawPath, tt.raw)
			}
			if got.Kind != tt.kind || got.Type != tt.typ {
				t.Fatalf("Kind/Type = %s/%s, want %s/%s", got.Kind, got.Type, tt.kind, tt.typ)
			}
			if got.RepeatableID != tt.id || got.RepeatableSlug != tt.slug || got.Index != tt.index {
				t.Fatalf("repeatable = %s %s [%d], want %s %s [%d]", got.RepeatableID, got.RepeatableSlug, got.Index, tt.id, tt.slug, tt.index)
			}
			if got.ParentCanvasRaw != tt.parent {
				t.Fatalf("ParentCanvasRaw = %q, want %q", got.ParentCanvasRaw, tt.parent)
			}
			if got.Value == nil {
				t.Fatal("expected Value subtree")
			}
			if tt.kind == KindRepeatable && len(got.Siblings) != expectedSiblingCount(tt.parent) {
				t.Fatalf("siblings = %d", len(got.Siblings))
			}
		})
	}
}

func expectedSiblingCount(parent string) int {
	switch parent {
	case "/items/Blokken":
		return 5
	case "/items/Blokken/repeatables/000000000000000000000003/items/Photos":
		return 2
	default:
		return 0
	}
}

func TestResolveErrorFormats(t *testing.T) {
	page := loadPage(t)
	schema := loadSchema(t)

	const indexErr = `path "Blokken[7].Title": Blokken has 5 repeatables (indexes 0-4):
  [0] hero_stage 000000000000000000000001
  [1] proof_strip 000000000000000000000002
  [2] case_cards 000000000000000000000003
  [3] case_cards 000000000000000000000004
  [4] case_cards 000000000000000000000005`

	const missingInRepeatable = "path \"Blokken[0].Titel\": editable \"Titel\" not found in Blokken[0] (hero_stage); editables:\n  CTA (field), Eyebrow (field), Footnote (field), Layout (select), Left Button - Link (field), Swoosh Bottom (hero) (file), Title (text)"

	const missingInRepeatableNoSchema = "path \"Blokken[0].Titel\": editable \"Titel\" not found in Blokken[0] (hero_stage); editables:\n  Eyebrow (field), Footnote (field), Left Button - Link (field), Swoosh Bottom (hero) (file), Title (text)"

	const missingOnPage = "path \"Blocks\": editable \"Blocks\" not found on page; editables:\n  Blokken (canvas), Korte uitleg \u2013 vraag (text), og_image (page field), published (page field), seo_description (page field), seo_keywords (page field), seo_title (page field), slug (page field), template (page field), title (page field)"

	const ambiguousSlug = `path "Blokken[slug=case_cards]": slug "case_cards" matches 3 repeatables; use an index or id=:
  [2] case_cards 000000000000000000000003, [3] case_cards 000000000000000000000004, [4] case_cards 000000000000000000000005`

	const missingID = `path "Blokken[id=abc]": repeatable id "abc" not found in Blokken; repeatables: [0] hero_stage 000000000000000000000001, [1] proof_strip 000000000000000000000002, [2] case_cards 000000000000000000000003, [3] case_cards 000000000000000000000004, [4] case_cards 000000000000000000000005`

	const depthErr = `path "Blokken[2].Photos[0].Nested[0].X": path exceeds two canvas levels`

	const emptyCanvasNoSchema = `path "Blokken[0].Title": Blokken has no repeatables yet; add one with: nimbu pages insert --page <page> --path Blokken --slug <slug>`

	const emptyCanvasWithSchema = emptyCanvasNoSchema + `; allowed slugs: hero_stage, proof_strip, case_cards`

	const emptyNestedCanvas = `path "Blokken[0].Photos[0]": Photos has no repeatables yet; add one with: nimbu pages insert --page <page> --path "Blokken[0].Photos" --slug <slug>`

	tests := []struct {
		name       string
		input      string
		schema     *Schema
		page       map[string]any
		want       string
		kind       string
		candidates int
	}{
		{name: "index out of range", input: "Blokken[7].Title", want: indexErr, kind: ResolveKindIndex, candidates: 5},
		{name: "unknown editable uses schema names", input: "Blokken[0].Titel", schema: schema, want: missingInRepeatable, kind: ResolveKindEditable, candidates: 7},
		{name: "unknown editable without schema uses doc", input: "Blokken[0].Titel", want: missingInRepeatableNoSchema, kind: ResolveKindEditable, candidates: 5},
		{name: "unknown page editable", input: "Blocks", want: missingOnPage, kind: ResolveKindPage, candidates: 10},
		{name: "ambiguous slug", input: "Blokken[slug=case_cards]", want: ambiguousSlug, kind: ResolveKindSlug, candidates: 3},
		{name: "unknown id", input: "Blokken[id=abc]", want: missingID, kind: ResolveKindID, candidates: 5},
		{name: "depth limit", input: "Blokken[2].Photos[0].Nested[0].X", want: depthErr, kind: ResolveKindDepth, candidates: 0},
		{
			name:   "empty canvas with schema",
			input:  "Blokken[0].Title",
			schema: schema,
			page:   emptyCanvasPage(),
			want:   emptyCanvasWithSchema,
			kind:   ResolveKindIndex,
		},
		{
			name:  "empty canvas without schema",
			input: "Blokken[0].Title",
			page:  emptyCanvasPage(),
			want:  emptyCanvasNoSchema,
			kind:  ResolveKindIndex,
		},
		{
			name:  "empty nested canvas uses full human path",
			input: "Blokken[0].Photos[0]",
			page:  emptyNestedCanvasPage(),
			want:  emptyNestedCanvas,
			kind:  ResolveKindIndex,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Parse(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			doc := page
			if tt.page != nil {
				doc = tt.page
			}
			_, err = Resolve(p, doc, tt.schema)
			if err == nil {
				t.Fatal("expected resolve error")
			}
			var rerr *ResolveError
			if !errors.As(err, &rerr) {
				t.Fatalf("error type %T, want *ResolveError", err)
			}
			if err.Error() != tt.want {
				t.Fatalf("error =\n%s\nwant\n%s", err.Error(), tt.want)
			}
			if rerr.Kind != tt.kind {
				t.Fatalf("Kind = %q, want %q", rerr.Kind, tt.kind)
			}
			if len(rerr.Candidates) != tt.candidates {
				t.Fatalf("Candidates = %v (len %d), want %d", rerr.Candidates, len(rerr.Candidates), tt.candidates)
			}
		})
	}
}

func TestResolveSiblingsFollowPositionOrder(t *testing.T) {
	page := loadPage(t)
	p, err := Parse("Blokken[0]")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(p, page, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []RepeatableRef{
		{ID: "000000000000000000000001", Slug: "hero_stage", Index: 0},
		{ID: "000000000000000000000002", Slug: "proof_strip", Index: 1},
		{ID: "000000000000000000000003", Slug: "case_cards", Index: 2},
		{ID: "000000000000000000000004", Slug: "case_cards", Index: 3},
		{ID: "000000000000000000000005", Slug: "case_cards", Index: 4},
	}
	if len(got.Siblings) != len(want) {
		t.Fatalf("siblings = %#v", got.Siblings)
	}
	for i := range want {
		if got.Siblings[i] != want[i] {
			t.Fatalf("sibling[%d] = %#v, want %#v", i, got.Siblings[i], want[i])
		}
	}
}

func TestEscapeNameRoundTripEnDash(t *testing.T) {
	name := "Korte uitleg \u2013 vraag"
	escaped := EscapeName(name)
	if escaped != "Korte%20uitleg%20%E2%80%93%20vraag" {
		t.Fatalf("EscapeName = %q", escaped)
	}
	if escaped != "" && containsPlus(escaped) {
		t.Fatalf("must not emit +: %q", escaped)
	}
	got, err := UnescapeName(escaped)
	if err != nil {
		t.Fatal(err)
	}
	if got != name {
		t.Fatalf("round-trip = %q", got)
	}
}

func containsPlus(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '+' {
			return true
		}
	}
	return false
}

func emptyCanvasPage() map[string]any {
	return map[string]any{
		"items": map[string]any{
			"Blokken": map[string]any{
				"type":        "canvas",
				"repeatables": []any{},
			},
		},
	}
}

func emptyNestedCanvasPage() map[string]any {
	return map[string]any{
		"items": map[string]any{
			"Blokken": map[string]any{
				"type": "canvas",
				"repeatables": []any{
					map[string]any{
						"id":   "000000000000000000000001",
						"slug": "hero_stage",
						"items": map[string]any{
							"Photos": map[string]any{
								"type":        "canvas",
								"repeatables": []any{},
							},
						},
					},
				},
			},
		},
	}
}

func loadPage(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "page.json"))
	if err != nil {
		t.Fatal(err)
	}
	var page map[string]any
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	return page
}
