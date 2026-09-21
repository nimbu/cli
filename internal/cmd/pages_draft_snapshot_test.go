package cmd

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDraftSnapshotToDocument(t *testing.T) {
	snapshot := map[string]any{
		"title":     "Draft About",
		"seo_title": "SEO",
		"page_items": []any{
			map[string]any{
				"slug":    "Theme",
				"type":    "select",
				"content": "Light",
			},
			map[string]any{
				"_id":  "item-blokken",
				"slug": "Blokken",
				"type": "canvas",
				"repeatables": []any{
					map[string]any{
						"_id":      "rep-draft-only",
						"slug":     "hero_stage",
						"position": 1,
						"page_items": []any{
							map[string]any{"slug": "Title", "type": "text", "content": "Draft Hero"},
							map[string]any{
								"slug": "Photos",
								"type": "canvas",
								"repeatables": []any{
									map[string]any{
										"id":       "photo-1",
										"slug":     "photo",
										"position": 2,
										"page_items": []any{
											map[string]any{"slug": "Image", "type": "file", "content": "https://cdn.example.test/a.jpg"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	doc, ok := draftSnapshotToDocument(snapshot)
	if !ok {
		t.Fatal("expected conversion to succeed")
	}
	if doc["title"] != "Draft About" || doc["seo_title"] != "SEO" {
		t.Fatalf("page fields = %#v", doc)
	}
	items, _ := doc["items"].(map[string]any)
	theme, _ := items["Theme"].(map[string]any)
	if theme["type"] != "select" || theme["content"] != "Light" {
		t.Fatalf("Theme = %#v", theme)
	}
	blokken, _ := items["Blokken"].(map[string]any)
	reps, _ := blokken["repeatables"].([]any)
	if len(reps) != 1 {
		t.Fatalf("repeatables = %#v", reps)
	}
	rep, _ := reps[0].(map[string]any)
	if rep["id"] != "rep-draft-only" || rep["slug"] != "hero_stage" || rep["position"] != 1 {
		t.Fatalf("repeatable = %#v", rep)
	}
	repItems, _ := rep["items"].(map[string]any)
	title, _ := repItems["Title"].(map[string]any)
	if title["content"] != "Draft Hero" {
		t.Fatalf("Title = %#v", title)
	}
	photos, _ := repItems["Photos"].(map[string]any)
	photoReps, _ := photos["repeatables"].([]any)
	photo, _ := photoReps[0].(map[string]any)
	if photo["id"] != "photo-1" || photo["position"] != 2 {
		t.Fatalf("nested repeatable = %#v", photo)
	}
	photoItems, _ := photo["items"].(map[string]any)
	image, _ := photoItems["Image"].(map[string]any)
	if image["type"] != "file" {
		t.Fatalf("Image = %#v", image)
	}
}

func TestDraftSnapshotToDocumentRejectsUnusableShape(t *testing.T) {
	if _, ok := draftSnapshotToDocument(nil); ok {
		t.Fatal("nil snapshot should fail")
	}
	if _, ok := draftSnapshotToDocument(map[string]any{"title": "x"}); ok {
		t.Fatal("missing page_items should fail")
	}
	if _, ok := draftSnapshotToDocument(map[string]any{"page_items": "nope"}); ok {
		t.Fatal("non-array page_items should fail")
	}
}

func TestDraftSnapshotToDocumentJSONFixtureRoundTripKeys(t *testing.T) {
	raw := `{
		"title":"Hi",
		"page_items":[
			{"_id":"a1","slug":"Intro","type":"text","content":"Hello"}
		]
	}`
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		t.Fatal(err)
	}
	doc, ok := draftSnapshotToDocument(snapshot)
	if !ok {
		t.Fatal("expected conversion")
	}
	items := doc["items"].(map[string]any)
	intro := items["Intro"].(map[string]any)
	if !reflect.DeepEqual(intro["content"], "Hello") {
		t.Fatalf("intro = %#v", intro)
	}
}

func TestConvertSnapshotPageItemCarriesFileReferenceAndTranslations(t *testing.T) {
	// A real Rails draft snapshot item is the raw Mongoid document: it keeps the
	// CarrierWave source_* columns and a translations array, never the
	// serialized "file"/"reference" objects the live API emits.
	out, ok := convertSnapshotPageItem(map[string]any{
		"_id":                 "item-1",
		"slug":                "Image",
		"type":                "file",
		"position":            float64(2),
		"disabled":            false,
		"source":              "uploads/hero-shot.png",
		"source_content_type": "image/png",
		"source_size":         float64(4321),
		"source_width":        float64(800),
		"source_height":       float64(600),
		"source_version":      "v3",
		"source_checksum":     "abc123",
		"translations": []any{
			map[string]any{"locale": "nl", "content": "Hallo"},
			map[string]any{"locale": "en", "content": "Hello"},
		},
	})
	if !ok {
		t.Fatal("expected conversion")
	}
	if out["id"] != "item-1" || out["slug"] != "Image" || out["type"] != "file" {
		t.Fatalf("identity = %#v", out)
	}
	if out["position"] != float64(2) || out["disabled"] != false {
		t.Fatalf("passthrough keys = %#v", out)
	}
	file, _ := out["file"].(map[string]any)
	if file["filename"] != "hero-shot.png" || file["content_type"] != "image/png" ||
		file["size"] != float64(4321) || file["width"] != float64(800) ||
		file["height"] != float64(600) || file["version"] != "v3" || file["checksum"] != "abc123" {
		t.Fatalf("file = %#v", file)
	}
	for _, leaked := range []string{"source", "source_content_type", "source_size", "source_width"} {
		if _, bad := out[leaked]; bad {
			t.Fatalf("%s must not leak into the document: %#v", leaked, out)
		}
	}
	translations, _ := out["translations"].(map[string]any)
	nl, _ := translations["nl"].(map[string]any)
	en, _ := translations["en"].(map[string]any)
	if nl["content"] != "Hallo" || en["content"] != "Hello" {
		t.Fatalf("translations = %#v", translations)
	}
	if _, bad := en["locale"]; bad {
		t.Fatalf("locale must be the map key, not a field: %#v", en)
	}
}

func TestConvertSnapshotPageItemBuildsReferenceObject(t *testing.T) {
	single, ok := convertSnapshotPageItem(map[string]any{
		"slug":           "Ref",
		"type":           "reference",
		"reference_type": "destinations",
		"reference_id":   map[string]any{"$oid": "6a6c699f0655c6dcb4dd8550"},
	})
	if !ok {
		t.Fatal("expected conversion")
	}
	if single["reference_id"] != "6a6c699f0655c6dcb4dd8550" {
		t.Fatalf("reference_id should be flattened from $oid: %#v", single)
	}
	reference, _ := single["reference"].(map[string]any)
	if reference["id"] != "6a6c699f0655c6dcb4dd8550" || reference["type"] != "destinations" {
		t.Fatalf("reference = %#v", reference)
	}

	many, ok := convertSnapshotPageItem(map[string]any{
		"slug":           "Refs",
		"type":           "reference",
		"reference_type": "products",
		"reference_ids":  []any{"a1", map[string]any{"$oid": "b2"}},
	})
	if !ok {
		t.Fatal("expected conversion")
	}
	manyRef, _ := many["reference"].(map[string]any)
	ids, _ := manyRef["ids"].([]any)
	if len(ids) != 2 || ids[0] != "a1" || ids[1] != "b2" {
		t.Fatalf("reference ids = %#v", manyRef)
	}
}

func TestDraftSnapshotToDocumentConvertsPageTranslations(t *testing.T) {
	doc, ok := draftSnapshotToDocument(map[string]any{
		"title": "Over ons",
		"translations": []any{
			map[string]any{"locale": "nl", "title": "Over ons"},
			map[string]any{"locale": "en", "title": "About us"},
		},
		"page_items": []any{
			map[string]any{"slug": "Intro", "type": "text", "content": "Hi"},
		},
	})
	if !ok {
		t.Fatal("expected conversion")
	}
	translations, _ := doc["translations"].(map[string]any)
	en, _ := translations["en"].(map[string]any)
	if en["title"] != "About us" {
		t.Fatalf("page translations = %#v", doc["translations"])
	}
}

func TestConvertSnapshotPageItemsSkipsDisabledItemsAndRepeatables(t *testing.T) {
	items, ok := convertSnapshotPageItems([]any{
		map[string]any{"slug": "Title", "type": "text", "content": "Kept"},
		map[string]any{"slug": "Hidden", "type": "text", "content": "Dropped", "disabled": true},
		map[string]any{"slug": "Blokken", "type": "canvas", "repeatables": []any{
			map[string]any{"_id": "6a6d0123456789abcdef0001", "slug": "hero", "position": 0, "page_items": []any{}},
			map[string]any{"_id": "6a6d0123456789abcdef0002", "slug": "hero", "position": 1, "disabled": true, "page_items": []any{}},
		}},
	})
	if !ok {
		t.Fatal("expected conversion to succeed")
	}
	if _, present := items["Hidden"]; present {
		t.Fatal("disabled item should be omitted")
	}
	if _, present := items["Title"]; !present {
		t.Fatal("enabled item should be kept")
	}
	canvas, _ := items["Blokken"].(map[string]any)
	reps, _ := canvas["repeatables"].([]any)
	if len(reps) != 1 {
		t.Fatalf("expected 1 enabled repeatable, got %d", len(reps))
	}
}
