package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/pagepath"
)

func outlineFixture(t *testing.T) PageDocument {
	t.Helper()
	const raw = `{
		"id":"p1",
		"title":"About",
		"items":{
			"Gallery":{"type":"canvas","repeatables":[]},
			"Tricky.Name":{"type":"text","content":"quoted"},
			"Blokken":{
				"type":"canvas",
				"repeatables":[
					{"id":"second","slug":"proof","position":2,"items":{"Quote":{"type":"text","content":"Hi"}}},
					{"id":"first","slug":"hero","position":1,"items":{
						"Space Name":{"type":"text","content":"spaced"},
						"Ref":{"type":"reference","reference":{"id":"r1","label":"Partner"}}
					}}
				]
			}
		}
	}`
	var doc PageDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return doc
}

func TestPageOutlineOrdersAndBuildsPaths(t *testing.T) {
	entries := PageOutline(outlineFixture(t))

	var paths []string
	byPath := map[string]PageOutlineEntry{}
	for _, entry := range entries {
		paths = append(paths, entry.Path)
		byPath[entry.Path] = entry
	}

	want := []string{
		"title",
		"Blokken",
		"Blokken[0]",
		"Blokken[0].Ref",
		"Blokken[0].Space Name",
		"Blokken[1]",
		"Blokken[1].Quote",
		"Gallery",
		"\"Tricky.Name\"",
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %q, want %q (all: %#v)", i, paths[i], want[i], paths)
		}
	}

	// Repeatables are ordered by position, not by document order.
	if byPath["Blokken[0]"].ID != "first" || byPath["Blokken[1]"].ID != "second" {
		t.Fatalf("repeatables not ordered by position: %#v", byPath["Blokken[0]"])
	}
	if got := byPath["Blokken[0].Space Name"].RawPath; got != "/items/Blokken/repeatables/first/items/Space%20Name" {
		t.Fatalf("raw path = %q", got)
	}
	if byPath["Gallery"].Repeatables != 0 || byPath["Gallery"].Type != PageOutlineTypeCanvas {
		t.Fatalf("empty canvas entry = %#v", byPath["Gallery"])
	}
	if byPath["Blokken[0].Ref"].Content == nil {
		t.Fatalf("reference content should fall back to the reference object")
	}
	if byPath["Blokken[0]"].Depth != 0 || byPath["Blokken[0].Ref"].Depth != 1 {
		t.Fatalf("unexpected depths: %#v %#v", byPath["Blokken[0]"], byPath["Blokken[0].Ref"])
	}
}

func TestPageCompactDocumentLeavesSourceUntouched(t *testing.T) {
	const raw = `{
		"id":"p1",
		"title":"About",
		"items":{
			"Hero":{
				"type":"file",
				"slug":"Hero",
				"created_at":"a",
				"updated_at":"b",
				"file":{"url":"https://cdn.example.test/a.png","filename":"a.png","width":1,"height":2,"size":3},
				"translations":{"nl":{"slug":"Hero","type":"file"}}
			}
		}
	}`
	var doc PageDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	compact, err := PageCompactDocument(doc)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	hero := compact["items"].(map[string]any)["Hero"].(map[string]any)
	if _, ok := hero["created_at"]; ok {
		t.Fatalf("created_at should be dropped: %#v", hero)
	}
	if _, ok := hero["translations"]; ok {
		t.Fatalf("empty translations should be dropped: %#v", hero)
	}
	file := hero["file"].(map[string]any)
	if _, ok := file["size"]; ok {
		t.Fatalf("file should be reduced: %#v", file)
	}

	source := doc["items"].(map[string]any)["Hero"].(map[string]any)
	if _, ok := source["created_at"]; !ok {
		t.Fatalf("source document was mutated: %#v", source)
	}
}

func TestPageProjectFields(t *testing.T) {
	doc := PageDocument{"id": "p1", "title": "About", "items": map[string]any{"Hero": map[string]any{}}}

	got, err := PageProjectFields(doc, []string{"id", "items"})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(got) != 2 || got["id"] != "p1" {
		t.Fatalf("projection = %#v", got)
	}
	if _, err := PageProjectFields(doc, []string{"nope"}); err == nil {
		t.Fatalf("expected unknown field error")
	}
}

// deepOutlineFixture nests three canvas levels and shadows two page fields
// with top-level editables.
func deepOutlineFixture(t *testing.T) PageDocument {
	t.Helper()
	const raw = `{
		"id":"p1",
		"title":"About",
		"og_image":{"url":"https://cdn.example.test/og.png","filename":"og.png"},
		"security_mechanism":"none",
		"items":{
			"title":{"type":"text","content":"shadowing editable"},
			"template":{"type":"canvas","repeatables":[
				{"id":"t1","slug":"block","position":1,"items":{"Inner":{"type":"text","content":"x"}}}
			]},
			"Blokken":{"type":"canvas","repeatables":[
				{"id":"b1","slug":"hero","position":1,"items":{
					"Photos":{"type":"canvas","repeatables":[
						{"id":"p1r","slug":"photo","position":1,"items":{
							"Deep":{"type":"canvas","repeatables":[
								{"id":"d1","slug":"deep","position":1,"items":{
									"Leaf":{"type":"text","content":"too deep"}
								}}
							]}
						}}
					]}
				}}
			]}
		}
	}`
	var doc PageDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return doc
}

func outlineByRawPath(entries []PageOutlineEntry) map[string]PageOutlineEntry {
	byRaw := make(map[string]PageOutlineEntry, len(entries))
	for _, entry := range entries {
		byRaw[entry.RawPath] = entry
	}
	return byRaw
}

// Regression: --outline advertised og_image/security_mechanism inconsistently
// with what pagepath accepts as a page field.
func TestPageOutlineListsAllPageFields(t *testing.T) {
	entries := PageOutline(deepOutlineFixture(t))
	byRaw := outlineByRawPath(entries)

	for _, field := range []string{"/title", "/og_image", "/security_mechanism"} {
		entry, ok := byRaw[field]
		if !ok {
			t.Fatalf("missing page field entry %q", field)
		}
		if entry.Type != PageOutlineTypePage || entry.Path != strings.TrimPrefix(field, "/") {
			t.Fatalf("page field entry %q = %#v", field, entry)
		}
		if _, err := pagepath.Parse(entry.Path); err != nil {
			t.Fatalf("page field path %q does not parse: %v", entry.Path, err)
		}
	}
}

// Regression: the outline recursed without limit, handing out human paths that
// pagepath.Resolve rejects past two canvas levels.
func TestPageOutlineDropsHumanPathPastTwoCanvasLevels(t *testing.T) {
	byRaw := outlineByRawPath(PageOutline(deepOutlineFixture(t)))

	shallow := map[string]string{
		"/items/Blokken/repeatables/b1":                                         "Blokken[0]",
		"/items/Blokken/repeatables/b1/items/Photos":                            "Blokken[0].Photos",
		"/items/Blokken/repeatables/b1/items/Photos/repeatables/p1r":            "Blokken[0].Photos[0]",
		"/items/Blokken/repeatables/b1/items/Photos/repeatables/p1r/items/Deep": "Blokken[0].Photos[0].Deep",
	}
	for raw, want := range shallow {
		entry, ok := byRaw[raw]
		if !ok {
			t.Fatalf("missing entry %q", raw)
		}
		if entry.Path != want {
			t.Fatalf("path for %q = %q, want %q", raw, entry.Path, want)
		}
	}

	deep := []string{
		"/items/Blokken/repeatables/b1/items/Photos/repeatables/p1r/items/Deep/repeatables/d1",
		"/items/Blokken/repeatables/b1/items/Photos/repeatables/p1r/items/Deep/repeatables/d1/items/Leaf",
	}
	for _, raw := range deep {
		entry, ok := byRaw[raw]
		if !ok {
			t.Fatalf("entry %q should still be listed", raw)
		}
		if entry.Path != "" {
			t.Fatalf("entry %q should have no human path, got %q", raw, entry.Path)
		}
	}
}

// Regression: an editable named like a page field got the page field's path,
// which pagepath.Parse resolves to the page field instead of the editable.
func TestPageOutlineDropsHumanPathOnPageFieldCollision(t *testing.T) {
	byRaw := outlineByRawPath(PageOutline(deepOutlineFixture(t)))

	collisions := []string{
		"/items/title",
		"/items/template",
		"/items/template/repeatables/t1",
		"/items/template/repeatables/t1/items/Inner",
	}
	for _, raw := range collisions {
		entry, ok := byRaw[raw]
		if !ok {
			t.Fatalf("missing entry %q", raw)
		}
		if entry.Path != "" {
			t.Fatalf("entry %q collides with a page field and must have no human path, got %q", raw, entry.Path)
		}
	}
}

// Every human path the outline advertises must parse and resolve back to the
// raw path it was printed next to.
func TestPageOutlinePathsRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		doc  PageDocument
	}{
		{name: "simple", doc: outlineFixture(t)},
		{name: "deep and shadowed", doc: deepOutlineFixture(t)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries := PageOutline(tc.doc)
			checked := 0
			for _, entry := range entries {
				if entry.Path == "" || entry.Type == PageOutlineTypeCanvas {
					continue
				}
				parsed, err := pagepath.Parse(entry.Path)
				if err != nil {
					t.Fatalf("parse %q: %v", entry.Path, err)
				}
				resolved, err := pagepath.Resolve(parsed, tc.doc, nil)
				if err != nil {
					t.Fatalf("resolve %q: %v", entry.Path, err)
				}
				if resolved.RawPath != entry.RawPath {
					t.Fatalf("path %q resolved to %q, want %q", entry.Path, resolved.RawPath, entry.RawPath)
				}
				checked++
			}
			if checked == 0 {
				t.Fatalf("no entries checked")
			}
		})
	}
}

// Regression: --compact stripped the attachment_path that --download-assets
// had just written, so the compacted document no longer round-tripped.
func TestPageCompactDocumentKeepsWritePathFileKeys(t *testing.T) {
	const raw = `{
		"og_image":{"url":"https://cdn.example.test/og.png","attachment_path":"assets/og.png","filename":"og.png","size":9},
		"items":{
			"Hero":{
				"type":"file",
				"file":{"attachment_path":"assets/a.png","filename":"a.png","size":3,"content_type":"image/png"}
			},
			"Ref":{
				"type":"file",
				"file":{"__type":"FileRef","source":"nimbu://abc","filename":"b.png","size":4}
			}
		}
	}`
	var doc PageDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	compact, err := PageCompactDocument(doc)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	items := compact["items"].(map[string]any)
	hero := items["Hero"].(map[string]any)["file"].(map[string]any)
	if hero["attachment_path"] != "assets/a.png" {
		t.Fatalf("attachment_path must survive compaction: %#v", hero)
	}
	if _, ok := hero["size"]; ok {
		t.Fatalf("compaction should still drop noise keys: %#v", hero)
	}
	ref := items["Ref"].(map[string]any)["file"].(map[string]any)
	if ref["__type"] != "FileRef" || ref["source"] != "nimbu://abc" {
		t.Fatalf("FileRef must survive compaction: %#v", ref)
	}
	og := compact["og_image"].(map[string]any)
	if og["attachment_path"] != "assets/og.png" {
		t.Fatalf("og_image attachment_path must survive compaction: %#v", og)
	}
}
